package revisar

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// consultorFalso cumple Consultor sin tocar la red. Vive en un archivo _test.go:
// existe para las pruebas y no viaja en el programa que se compila.
type consultorFalso struct {
	codigos map[string]int // nombre -> código que «contesta»; si falta, 200
	espera  time.Duration  // latencia simulada para cada consulta

	mu         sync.Mutex
	enCurso    int
	maxEnCurso int
	llamadas   atomic.Int32
}

func (f *consultorFalso) Consultar(ctx context.Context, s servicio.Servicio) servicio.Estado {
	f.llamadas.Add(1)
	f.mu.Lock()
	f.enCurso++
	if f.enCurso > f.maxEnCurso {
		f.maxEnCurso = f.enCurso
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.enCurso--; f.mu.Unlock() }()

	if f.espera > 0 {
		select {
		case <-time.After(f.espera):
		case <-ctx.Done():
			return servicio.Estado{Servicio: s, Err: traducir(ctx, ctx.Err())}
		}
	}
	codigo, ok := f.codigos[s.Nombre]
	if !ok {
		codigo = 200
	}
	return servicio.Estado{Servicio: s, Codigo: codigo}
}

func servicios(nombres ...string) []servicio.Servicio {
	var ss []servicio.Servicio
	for _, n := range nombres {
		ss = append(ss, servicio.Servicio{Nombre: n, URL: "http://" + n + ".invalid"})
	}
	return ss
}

func TestTodos(t *testing.T) {
	casos := []struct {
		nombre    string
		servicios []servicio.Servicio
		codigos   map[string]int
		quieroOK  []bool
	}{
		{"lista vacía", nil, nil, []bool{}},
		{"todos sanos", servicios("a", "b", "c"), nil, []bool{true, true, true}},
		{"uno caído en medio", servicios("a", "b", "c"), map[string]int{"b": 503}, []bool{true, false, true}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &consultorFalso{codigos: c.codigos}
			got := Todos(context.Background(), f, c.servicios, 0)
			if len(got) != len(c.quieroOK) {
				t.Fatalf("len = %d, quiero %d", len(got), len(c.quieroOK))
			}
			for i, e := range got {
				if e.Servicio.Nombre != c.servicios[i].Nombre {
					t.Errorf("posición %d: %q, quiero %q (el orden de entrada se respeta)", i, e.Servicio.Nombre, c.servicios[i].Nombre)
				}
				if e.OK() != c.quieroOK[i] {
					t.Errorf("%s: OK() = %v, quiero %v", e.Servicio.Nombre, e.OK(), c.quieroOK[i])
				}
			}
			if int(f.llamadas.Load()) != len(c.servicios) {
				t.Errorf("llamadas = %d, quiero %d", f.llamadas.Load(), len(c.servicios))
			}
		})
	}
}

func TestTodosRespetaElParalelo(t *testing.T) {
	f := &consultorFalso{espera: 20 * time.Millisecond}
	Todos(context.Background(), f, servicios("a", "b", "c", "d", "e", "f", "g", "h"), 3)
	if f.maxEnCurso > 3 {
		t.Errorf("hubo %d consultas a la vez, el máximo era 3", f.maxEnCurso)
	}
	if f.maxEnCurso < 2 {
		t.Errorf("hubo %d consultas a la vez: no se está consultando en paralelo", f.maxEnCurso)
	}
}

func TestTodosCortaAlServicioLento(t *testing.T) {
	f := &consultorFalso{espera: time.Second}
	lento := []servicio.Servicio{{Nombre: "lento", URL: "http://lento.invalid", Timeout: 30 * time.Millisecond}}

	inicio := time.Now()
	got := Todos(context.Background(), f, lento, 0)
	if d := time.Since(inicio); d > 500*time.Millisecond {
		t.Fatalf("tardó %v: el límite de 30ms no se aplicó", d)
	}
	if got[0].OK() || got[0].Motivo() != "se acabó el tiempo de espera" {
		t.Errorf("estado = %+v, quiero el aviso de tiempo agotado", got[0])
	}
}
