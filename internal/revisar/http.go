package revisar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// limiteDrenado es cuánto del cuerpo de una respuesta se lee, a lo más, para
// tirarlo.
const limiteDrenado = 64 << 10 // 64 KiB

// HTTP consulta un servicio con una petición GET y mira qué contesta.
//
// Es un tipo concreto, no una interfaz: quien lo quiera sustituir en una prueba
// declara su propia interfaz del lado que lo usa (como hace Todos con
// Consultor).
type HTTP struct {
	cliente *http.Client
}

// NuevoHTTP arma el consultor con un cliente propio.
//
// El cliente es propio y no http.DefaultClient a propósito: el de omisión no
// tiene tiempo límite, y una petición a un servicio que acepta la conexión y
// nunca contesta se quedaría colgada para siempre. El límite fino, uno por
// servicio, lo pone Todos con el context; éste es la red de seguridad.
func NuevoHTTP(limiteGeneral time.Duration) *HTTP {
	return &HTTP{cliente: &http.Client{Timeout: limiteGeneral}}
}

// Consultar cumple la interfaz Consultor.
func (h *HTTP) Consultar(ctx context.Context, s servicio.Servicio) servicio.Estado {
	inicio := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return servicio.Estado{Servicio: s, Duracion: time.Since(inicio),
			Err: fmt.Errorf("URL inválida: %w", sinURL(err))}
	}

	resp, err := h.cliente.Do(req)
	if err != nil {
		return servicio.Estado{Servicio: s, Duracion: time.Since(inicio), Err: traducir(ctx, err)}
	}
	// Se cierra después de revisar el error, nunca antes: si err != nil, resp es
	// nil y cerrarlo sería un pánico.
	defer resp.Body.Close()

	// Se drena el cuerpo aunque no se use: sin esto la conexión no se reutiliza.
	// Con tope: un servicio que contesta 200 con un cuerpo de gigabytes, o sin
	// fin, nos tendría leyendo hasta agotar el tiempo límite. Si el cuerpo pasa
	// del tope, Close corta la conexión en vez de reutilizarla, y está bien.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, limiteDrenado))

	return servicio.Estado{Servicio: s, Codigo: resp.StatusCode, Duracion: time.Since(inicio)}
}

// traducir cambia el error de net/http por uno corto que se entienda en un
// reporte.
func traducir(ctx context.Context, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return errors.New("se acabó el tiempo de espera")
	case errors.Is(ctx.Err(), context.Canceled):
		return errors.New("consulta cancelada")
	default:
		return fmt.Errorf("no responde: %w", sinURL(err))
	}
}

// sinURL quita la URL de un error de net/http o de net/url.
//
// 🔒 Esos errores se imprimen como «Get "https://host/ruta?token=...": causa»:
// llevan la URL entera, query string incluido, y el motivo sale en la API.
// Nos quedamos solo con la causa.
func sinURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
