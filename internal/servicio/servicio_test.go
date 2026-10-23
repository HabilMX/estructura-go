package servicio

import (
	"errors"
	"testing"
	"time"
)

func TestTimeoutEfectivo(t *testing.T) {
	casos := []struct {
		nombre string
		dado   time.Duration
		quiero time.Duration
	}{
		{"sin timeout usa el de omisión", 0, TimeoutPorOmision},
		{"negativo usa el de omisión", -1 * time.Second, TimeoutPorOmision},
		{"uno declarado se respeta", 500 * time.Millisecond, 500 * time.Millisecond},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := Servicio{Nombre: "x", URL: "http://x", Timeout: c.dado}
			if got := s.TimeoutEfectivo(); got != c.quiero {
				t.Errorf("TimeoutEfectivo() = %v, quiero %v", got, c.quiero)
			}
		})
	}
}

func TestEstadoOKyMotivo(t *testing.T) {
	casos := []struct {
		nombre     string
		estado     Estado
		quieroOK   bool
		quieroPorq string
	}{
		{"200 está sano", Estado{Codigo: 200}, true, ""},
		{"204 también", Estado{Codigo: 204}, true, ""},
		{"500 no", Estado{Codigo: 500}, false, "respondió 500"},
		{"404 tampoco", Estado{Codigo: 404}, false, "respondió 404"},
		{"sin código ni error", Estado{}, false, "sin respuesta"},
		{"el error manda aunque haya código", Estado{Codigo: 200, Err: errors.New("se cortó")}, false, "se cortó"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.estado.OK(); got != c.quieroOK {
				t.Errorf("OK() = %v, quiero %v", got, c.quieroOK)
			}
			if got := c.estado.Motivo(); got != c.quieroPorq {
				t.Errorf("Motivo() = %q, quiero %q", got, c.quieroPorq)
			}
		})
	}
}
