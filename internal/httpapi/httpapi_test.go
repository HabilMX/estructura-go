package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// consultorFijo contesta siempre lo mismo según el nombre. Cumple
// revisar.Consultor sin que httpapi necesite saber cómo se consulta de verdad.
type consultorFijo map[string]int

func (c consultorFijo) Consultar(_ context.Context, s servicio.Servicio) servicio.Estado {
	return servicio.Estado{Servicio: s, Codigo: c[s.Nombre], Duracion: 7 * time.Millisecond}
}

func nuevoServidor() http.Handler {
	return Nuevo(Opciones{
		Servicios: []servicio.Servicio{
			{Nombre: "pagos", URL: "http://pagos.invalid"},
			{Nombre: "correo", URL: "http://correo.invalid"},
		},
		Consultor: consultorFijo{"pagos": 200, "correo": 503},
		Paralelo:  2,
		Limite:    time.Second,
	})
}

func pedir(t *testing.T, h http.Handler, metodo, ruta string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(metodo, ruta, nil))
	return rec
}

func TestRutas(t *testing.T) {
	casos := []struct {
		nombre     string
		metodo     string
		ruta       string
		quieroCod  int
		quieroTipo string
	}{
		{"salud", "GET", "/salud", 200, "application/json"},
		{"lista", "GET", "/servicios", 200, "application/json"},
		{"uno que existe", "GET", "/servicios/pagos", 200, "application/json"},
		{"uno que no existe", "GET", "/servicios/nomina", 404, "application/problem+json"},
		{"método equivocado", "POST", "/servicios", 405, ""},
	}
	h := nuevoServidor()
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := pedir(t, h, c.metodo, c.ruta)
			if rec.Code != c.quieroCod {
				t.Errorf("código = %d, quiero %d", rec.Code, c.quieroCod)
			}
			if c.quieroTipo != "" && rec.Header().Get("Content-Type") != c.quieroTipo {
				t.Errorf("Content-Type = %q, quiero %q", rec.Header().Get("Content-Type"), c.quieroTipo)
			}
		})
	}
}

func TestListaConservaElOrdenYElEstado(t *testing.T) {
	rec := pedir(t, nuevoServidor(), "GET", "/servicios")
	var got []estadoJSON
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	quiero := []estadoJSON{
		{Nombre: "pagos", URL: "http://pagos.invalid", OK: true, Codigo: 200, DuracionMs: 7},
		{Nombre: "correo", URL: "http://correo.invalid", OK: false, Codigo: 503, DuracionMs: 7, Motivo: "respondió 503"},
	}
	if len(got) != len(quiero) {
		t.Fatalf("salieron %d servicios, quiero %d", len(got), len(quiero))
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("posición %d = %+v, quiero %+v", i, got[i], quiero[i])
		}
	}
}

func TestNoEncontradoEsProblemDetails(t *testing.T) {
	rec := pedir(t, nuevoServidor(), "GET", "/servicios/nomina")
	var p problema
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	quiero := problema{
		Type:     "about:blank",
		Title:    "Not Found",
		Status:   404,
		Detail:   `No hay ningún servicio llamado "nomina".`,
		Instance: "/servicios/nomina",
	}
	if p != quiero {
		t.Errorf("problema = %+v\nquiero     %+v", p, quiero)
	}
}

// TestLaURLPublicaNoLlevaSecretos: aunque una URL con credenciales o con un
// token en el query string llegue hasta aquí (config rechaza la primera, no la
// segunda), lo que sale por la API es solo esquema, servidor y ruta.
func TestLaURLPublicaNoLlevaSecretos(t *testing.T) {
	h := Nuevo(Opciones{
		Servicios: []servicio.Servicio{
			{Nombre: "pagos", URL: "https://yo:supersecreto@pagos.invalid/estado?token=abc123#ancla"},
		},
		Consultor: consultorFijo{"pagos": 200},
		Paralelo:  1,
		Limite:    time.Second,
	})
	for _, ruta := range []string{"/servicios", "/servicios/pagos"} {
		cuerpo := pedir(t, h, "GET", ruta).Body.String()
		for _, secreto := range []string{"supersecreto", "abc123", "yo:", "ancla"} {
			if strings.Contains(cuerpo, secreto) {
				t.Errorf("%s filtra %q: %s", ruta, secreto, cuerpo)
			}
		}
		if !strings.Contains(cuerpo, `"url":"https://pagos.invalid/estado"`) {
			t.Errorf("%s: quiero la URL limpia https://pagos.invalid/estado; salió %s", ruta, cuerpo)
		}
	}
}

// consultorQueEspera no contesta hasta que le cancelan el context, como un
// servicio colgado. Así se ve quién pone el plazo.
type consultorQueEspera struct{}

func (consultorQueEspera) Consultar(ctx context.Context, s servicio.Servicio) servicio.Estado {
	<-ctx.Done()
	return servicio.Estado{Servicio: s, Err: ctx.Err()}
}

// TestUnaRevisionNoPasaDelLimite: Opciones.Limite es el tiempo máximo de una
// revisión completa, aunque cada servicio declare un plazo mucho mayor y la
// petición no traiga ninguno. Sin el WithTimeout de api.revisar, la respuesta
// no llega nunca.
func TestUnaRevisionNoPasaDelLimite(t *testing.T) {
	h := Nuevo(Opciones{
		Servicios: []servicio.Servicio{{Nombre: "colgado", URL: "http://colgado.invalid", Timeout: time.Hour}},
		Consultor: consultorQueEspera{},
		Paralelo:  1,
		Limite:    50 * time.Millisecond,
	})
	termino := make(chan *httptest.ResponseRecorder, 1)
	go func() { termino <- pedir(t, h, "GET", "/servicios/colgado") }()
	select {
	case rec := <-termino:
		if !strings.Contains(rec.Body.String(), `"ok":false`) {
			t.Errorf("un servicio que no contestó a tiempo no puede salir sano: %s", rec.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la revisión sigue corriendo a los 5 s: Opciones.Limite no se aplicó")
	}
}
