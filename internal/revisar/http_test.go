package revisar

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// TestHTTPConsultar prueba el consultor de verdad contra un servidor local que
// levanta httptest: hay red, pero solo dentro de la máquina.
func TestHTTPConsultar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/falla", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/lento", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	casos := []struct {
		nombre       string
		ruta         string
		timeout      time.Duration
		quieroCodigo int
		quieroMotivo string
	}{
		{"sano", "/ok", 0, 200, ""},
		{"responde 500", "/falla", 0, 500, "respondió 500"},
		{"no contesta a tiempo", "/lento", 50 * time.Millisecond, 0, "se acabó el tiempo de espera"},
	}
	h := NuevoHTTP(5 * time.Second)
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := servicio.Servicio{Nombre: c.nombre, URL: srv.URL + c.ruta, Timeout: c.timeout}
			ctx, cancelar := context.WithTimeout(context.Background(), s.TimeoutEfectivo())
			defer cancelar()

			e := h.Consultar(ctx, s)
			if e.Codigo != c.quieroCodigo || e.Motivo() != c.quieroMotivo {
				t.Errorf("código %d motivo %q; quiero %d y %q", e.Codigo, e.Motivo(), c.quieroCodigo, c.quieroMotivo)
			}
		})
	}
}

func TestHTTPConsultarURLInvalida(t *testing.T) {
	e := NuevoHTTP(time.Second).Consultar(context.Background(), servicio.Servicio{Nombre: "x", URL: "http://[::1"})
	if e.OK() || e.Err == nil {
		t.Fatalf("una URL mal formada tiene que fallar con error, salió %+v", e)
	}
}

// TestElMotivoNoLlevaLaURL: los errores de net/http traen la URL completa
// («Get "http://...?token=..."»), y el motivo sale por la API. Se comprueba con
// un servidor que acaba de cerrarse, para que la conexión falle de verdad.
func TestElMotivoNoLlevaLaURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/estado?token=abc123"
	srv.Close()

	e := NuevoHTTP(time.Second).Consultar(context.Background(), servicio.Servicio{Nombre: "x", URL: url})
	if e.OK() {
		t.Fatal("un servidor cerrado no puede salir sano")
	}
	if strings.Contains(e.Motivo(), "abc123") || strings.Contains(e.Motivo(), "/estado") {
		t.Errorf("el motivo filtra la URL: %q", e.Motivo())
	}
	if !strings.HasPrefix(e.Motivo(), "no responde: ") {
		t.Errorf("motivo = %q, quiero que empiece con «no responde: »", e.Motivo())
	}
}

// cuerpoContado es un cuerpo de respuesta enorme que cuenta cuántos bytes le
// leyeron. Se acaba a los 16 MiB para que la prueba termine aunque el drenado
// no tenga tope (si lo tuviera infinito, sin tope la prueba no acabaría nunca).
type cuerpoContado struct {
	leidos int64
}

const tamanoCuerpoEnorme = 16 << 20

func (c *cuerpoContado) Read(p []byte) (int, error) {
	if c.leidos >= tamanoCuerpoEnorme {
		return 0, io.EOF
	}
	n := int64(len(p))
	if resta := tamanoCuerpoEnorme - c.leidos; n > resta {
		n = resta
	}
	c.leidos += n
	return int(n), nil
}

func (c *cuerpoContado) Close() error { return nil }

// transporteFijo contesta 200 con el cuerpo que se le dé, sin red.
type transporteFijo struct{ cuerpo io.ReadCloser }

func (t transporteFijo) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: t.cuerpo, Header: http.Header{}, Request: r}, nil
}

// TestElDrenadoTieneTope: un servicio que contesta 200 con un cuerpo de
// gigabytes (o sin fin) no debe tenernos leyendo hasta que se acabe el tiempo.
// Se drena a lo más limiteDrenado bytes.
func TestElDrenadoTieneTope(t *testing.T) {
	cuerpo := &cuerpoContado{}
	h := &HTTP{cliente: &http.Client{Transport: transporteFijo{cuerpo}}}

	e := h.Consultar(context.Background(), servicio.Servicio{Nombre: "x", URL: "http://enorme.invalid/"})
	if !e.OK() {
		t.Fatalf("un 200 tiene que salir sano; salió %+v", e)
	}
	if cuerpo.leidos > limiteDrenado {
		t.Errorf("se leyeron %d bytes del cuerpo; el tope es %d", cuerpo.leidos, limiteDrenado)
	}
}

// TestElClienteTieneSuPropioTiempoLimite: aunque quien llama no ponga plazo en
// el context, una petición a un servicio que acepta la conexión y nunca
// contesta tiene que terminar. Es la «red de seguridad» de NuevoHTTP: con
// http.DefaultClient, que no tiene tiempo límite, esta prueba se pone roja.
func TestElClienteTieneSuPropioTiempoLimite(t *testing.T) {
	soltar := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-soltar:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(soltar) // corre antes que srv.Close: suelta al manejador colgado

	termino := make(chan servicio.Estado, 1)
	go func() {
		termino <- NuevoHTTP(50*time.Millisecond).Consultar(context.Background(), servicio.Servicio{Nombre: "x", URL: srv.URL})
	}()
	select {
	case e := <-termino:
		if e.OK() || e.Err == nil {
			t.Errorf("un servicio que nunca contesta no puede salir sano; salió %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la consulta sigue colgada a los 5 s: el cliente no tiene tiempo límite propio")
	}
}
