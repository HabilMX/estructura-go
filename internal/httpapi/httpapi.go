// Package httpapi es la puerta de entrada por HTTP: rutas, JSON y errores.
//
// No sabe consultar servicios (eso es de revisar) ni qué es un servicio sano
// (eso es de servicio). Solo traduce: de una petición HTTP a una llamada, y de
// un resultado a JSON. Los errores que escribe este paquete salen como Problem
// Details (RFC 9457). Los que contesta el enrutador por su cuenta (405 por
// método equivocado, 404 de una ruta que no existe) salen en texto plano.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/HabilMX/estructura-go/internal/revisar"
	"github.com/HabilMX/estructura-go/internal/servicio"
)

// Opciones es lo que el servidor necesita para revisar.
type Opciones struct {
	Servicios []servicio.Servicio
	Consultor revisar.Consultor
	Paralelo  int
	Limite    time.Duration // tiempo máximo de una revisión completa
}

// Nuevo devuelve el manejador con las tres rutas de la API.
func Nuevo(op Opciones) http.Handler {
	a := &api{op: op, porNombre: make(map[string]servicio.Servicio, len(op.Servicios))}
	for _, s := range op.Servicios {
		a.porNombre[s.Nombre] = s
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /salud", a.salud)
	mux.HandleFunc("GET /servicios", a.listar)
	mux.HandleFunc("GET /servicios/{nombre}", a.uno)
	return mux
}

type api struct {
	op        Opciones
	porNombre map[string]servicio.Servicio
}

// estadoJSON es lo que sale por la API. Vive aquí y no en servicio: el formato
// público es decisión de la puerta, no del dominio.
type estadoJSON struct {
	Nombre     string `json:"nombre"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	Codigo     int    `json:"codigo,omitempty"`
	DuracionMs int64  `json:"duracion_ms"`
	Motivo     string `json:"motivo,omitempty"`
}

func aJSON(e servicio.Estado) estadoJSON {
	return estadoJSON{
		Nombre:     e.Servicio.Nombre,
		URL:        urlPublica(e.Servicio.URL),
		OK:         e.OK(),
		Codigo:     e.Codigo,
		DuracionMs: e.Duracion.Milliseconds(),
		Motivo:     e.Motivo(),
	}
}

// urlPublica devuelve la URL lista para salir por la API: sin usuario ni
// contraseña, sin query string y sin fragmento.
//
// 🔒 config ya rechaza URLs con credenciales, pero esta puerta no confía en que
// todos los caminos pasen por config: un token en el query string
// (?token=...) es igual de secreto y config sí lo deja pasar, porque hay
// servicios que lo exigen para consultarse. Lo que se publica es solo esquema,
// servidor y ruta. Si la URL no se puede leer, no se publica nada.
//
// ⚠️ Límite, a propósito: la RUTA sí se publica, porque dice qué se revisa
// (/salud, /estado) y la mayoría de las rutas no son secretas. Un token metido
// en la ruta (como el de algunos webhooks, /hooks/T000/<token>) saldría entero:
// desde aquí no hay forma de distinguirlo de una ruta normal. Ese servicio no
// se debe revisar con su URL secreta; usa una ruta de salud sin token.
func urlPublica(crudo string) string {
	u, err := url.Parse(crudo)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.Redacted()
}

// salud responde que el propio servidor está vivo. No consulta a nadie: así el
// revisor se puede revisar a sí mismo sin llamarse en bucle.
func (a *api) salud(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]string{"estado": "ok"})
}

func (a *api) listar(w http.ResponseWriter, r *http.Request) {
	estados := a.revisar(r.Context(), a.op.Servicios)
	salida := make([]estadoJSON, 0, len(estados))
	for _, e := range estados {
		salida = append(salida, aJSON(e))
	}
	escribirJSON(w, http.StatusOK, salida)
}

func (a *api) uno(w http.ResponseWriter, r *http.Request) {
	nombre := r.PathValue("nombre")
	s, existe := a.porNombre[nombre]
	if !existe {
		escribirProblema(w, problema{
			Status:   http.StatusNotFound,
			Detail:   fmt.Sprintf("No hay ningún servicio llamado %q.", nombre),
			Instance: r.URL.Path,
		})
		return
	}
	estados := a.revisar(r.Context(), []servicio.Servicio{s})
	escribirJSON(w, http.StatusOK, aJSON(estados[0]))
}

func (a *api) revisar(ctx context.Context, ss []servicio.Servicio) []servicio.Estado {
	ctx, cancelar := context.WithTimeout(ctx, a.op.Limite)
	defer cancelar()
	return revisar.Todos(ctx, a.op.Consultor, ss, a.op.Paralelo)
}

// problema es el cuerpo de un error según la RFC 9457.
type problema struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// escribirProblema completa type y title como pide la RFC 9457 (4.2.1): con
// "about:blank", el título es la frase estándar del código, p. ej. "Not Found".
func escribirProblema(w http.ResponseWriter, p problema) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func escribirJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
