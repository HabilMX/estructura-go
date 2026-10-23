package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// bitacoraSegura deja que el servidor escriba y la prueba lea al mismo tiempo
// sin una carrera de datos (go test -race la señalaría con un bytes.Buffer solo).
type bitacoraSegura struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *bitacoraSegura) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *bitacoraSegura) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestArrancaContestaYSeApaga prueba el programa completo: arranca en un puerto
// libre, responde /salud y se apaga limpio cuando se cancela el context, igual
// que cuando alguien oprime Ctrl+C.
func TestArrancaContestaYSeApaga(t *testing.T) {
	lista := filepath.Join(t.TempDir(), "servicios.json")
	if err := os.WriteFile(lista, []byte(`[{"nombre":"a","url":"http://a.invalid"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	var bitacora bitacoraSegura
	termino := make(chan error, 1)
	go func() {
		termino <- ejecutar(ctx, []string{"-direccion", "127.0.0.1:0", "-servicios", lista}, func(string) string { return "" }, &bitacora)
	}()

	// El puerto lo eligió el sistema (":0"); se lee de la bitácora.
	var url string
	for intento := 0; intento < 100 && url == ""; intento++ {
		time.Sleep(10 * time.Millisecond)
		for _, campo := range strings.Fields(bitacora.String()) {
			if dir, ok := strings.CutPrefix(campo, "direccion="); ok {
				url = "http://" + dir + "/salud"
			}
		}
	}
	if url == "" {
		t.Fatalf("el servidor no anunció su dirección; bitácora: %s", bitacora.String())
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/salud = %d, quiero 200", resp.StatusCode)
	}

	cancelar()
	select {
	case err := <-termino:
		if err != nil {
			t.Errorf("al apagarse devolvió %v, quiero nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no se apagó en 5 segundos")
	}
}

func TestErroresDeArranque(t *testing.T) {
	casos := []struct {
		nombre    string
		args      []string
		quieroErr string
	}{
		{"archivo que no existe", []string{"-servicios", "no-existe.json"}, "abriendo la lista"},
		{"bandera inválida", []string{"-paralelo", "-1"}, "mayor que cero"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ejecutar(context.Background(), c.args, func(string) string { return "" }, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), c.quieroErr) {
				t.Fatalf("error = %v, quiero uno que diga %q", err, c.quieroErr)
			}
		})
	}
}
