package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func sinEntorno(string) string { return "" }

func TestDesde(t *testing.T) {
	casos := []struct {
		nombre    string
		args      []string
		entorno   map[string]string
		quiero    Config
		quieroErr string
	}{
		{
			nombre: "todo por omisión",
			quiero: Config{Direccion: ":8080", Servicios: "testdata/servicios.json", Paralelo: 5, Limite: 10 * time.Second},
		},
		{
			nombre:  "el entorno cambia la omisión",
			entorno: map[string]string{"REVISOR_DIRECCION": ":9000"},
			quiero:  Config{Direccion: ":9000", Servicios: "testdata/servicios.json", Paralelo: 5, Limite: 10 * time.Second},
		},
		{
			nombre:  "la bandera gana al entorno",
			args:    []string{"-direccion", ":7000", "-paralelo", "2"},
			entorno: map[string]string{"REVISOR_DIRECCION": ":9000"},
			quiero:  Config{Direccion: ":7000", Servicios: "testdata/servicios.json", Paralelo: 2, Limite: 10 * time.Second},
		},
		{nombre: "paralelo en cero", args: []string{"-paralelo", "0"}, quieroErr: "mayor que cero"},
		{nombre: "bandera que no existe", args: []string{"-puerto", "1"}, quieroErr: "banderas"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			entorno := sinEntorno
			if c.entorno != nil {
				entorno = func(k string) string { return c.entorno[k] }
			}
			got, err := Desde(c.args, entorno)
			if c.quieroErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.quieroErr) {
					t.Fatalf("error = %v, quiero uno que diga %q", err, c.quieroErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.quiero {
				t.Errorf("Desde() = %+v, quiero %+v", got, c.quiero)
			}
		})
	}
}

func TestLeerServicios(t *testing.T) {
	casos := []struct {
		nombre    string
		json      string
		quieroN   int
		quieroErr string
	}{
		{"dos válidos", `[{"nombre":"a","url":"http://a"},{"nombre":"b","url":"https://b","timeout":"500ms"}]`, 2, ""},
		{"lista vacía", `[]`, 0, "ningún servicio"},
		{"no es JSON", `nombre url`, 0, "no es una lista JSON"},
		{"campo mal escrito", `[{"nombre":"a","url":"http://a","timout":"1s"}]`, 0, "no es una lista JSON"},
		{"sin nombre", `[{"url":"http://a"}]`, 0, "servicio 1: falta el nombre"},
		{"nombre repetido", `[{"nombre":"a","url":"http://a"},{"nombre":"a","url":"http://b"}]`, 0, "ya estaba en el 1"},
		{"URL sin esquema", `[{"nombre":"a","url":"a.com"}]`, 0, "tiene que empezar con http"},
		{"URL sin servidor", `[{"nombre":"a","url":"http://"}]`, 0, "le falta el servidor"},
		{"URL con usuario y contraseña", `[{"nombre":"a","url":"https://yo:supersecreto@a.com"}]`, 0, "las credenciales no van en la URL"},
		{"URL con usuario sin contraseña", `[{"nombre":"a","url":"https://token123@a.com"}]`, 0, "las credenciales no van en la URL"},
		{"timeout ilegible", `[{"nombre":"a","url":"http://a","timeout":"rápido"}]`, 0, "timeout \"rápido\" inválido"},
		{"timeout negativo", `[{"nombre":"a","url":"http://a","timeout":"-1s"}]`, 0, "inválido"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := LeerServicios(strings.NewReader(c.json))
			if c.quieroErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.quieroErr) {
					t.Fatalf("error = %v, quiero uno que diga %q", err, c.quieroErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.quieroN {
				t.Errorf("leyó %d servicios, quiero %d", len(got), c.quieroN)
			}
		})
	}
}

// TestElErrorNoRepiteElSecreto: rechazar la URL no sirve de nada si el mensaje
// de error imprime la contraseña que estamos rechazando.
func TestElErrorNoRepiteElSecreto(t *testing.T) {
	_, err := LeerServicios(strings.NewReader(`[{"nombre":"a","url":"https://yo:supersecreto@a.com"}]`))
	if err == nil {
		t.Fatal("una URL con credenciales tiene que rechazarse")
	}
	if strings.Contains(err.Error(), "supersecreto") {
		t.Errorf("el error filtra la contraseña: %v", err)
	}
}

// TestElEjemploDelRepoEsValido lee el archivo que usa «Córrelo» en el README.
// Si alguien lo rompe, esta prueba lo dice antes que el primer lector.
func TestElEjemploDelRepoEsValido(t *testing.T) {
	f, err := os.Open("../../testdata/servicios.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := LeerServicios(f); err != nil {
		t.Fatalf("testdata/servicios.json no es válido: %v", err)
	}
}
