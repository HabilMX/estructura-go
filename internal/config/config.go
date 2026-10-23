// Package config reúne lo que el programa necesita saber antes de arrancar:
// dónde escuchar, qué servicios revisar y cuánto esperar.
//
// Hace dos cosas, y ninguna toca la red:
//
//   - Desde lee las banderas y las variables de entorno. Recibe la función que
//     lee el entorno como parámetro, así que se prueba sin tocar el entorno real.
//   - LeerServicios convierte el JSON de servicios en una lista validada.
//     Recibe un io.Reader, así que se prueba con texto y sin archivos.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// Config es todo lo que main necesita para armar el programa.
type Config struct {
	Direccion string        // dónde escucha el servidor, p. ej. ":8080"
	Servicios string        // ruta al archivo JSON de servicios
	Paralelo  int           // cuántos servicios se consultan a la vez
	Limite    time.Duration // tiempo máximo para una revisión completa
}

// Desde arma la configuración. Cada valor sale, en este orden, de la bandera,
// de la variable de entorno o del valor por omisión.
func Desde(args []string, entorno func(string) string) (Config, error) {
	f := flag.NewFlagSet("revisor", flag.ContinueOnError)
	f.SetOutput(io.Discard) // el error lo devuelve Desde; main decide cómo mostrarlo

	var c Config
	f.StringVar(&c.Direccion, "direccion", valor(entorno, "REVISOR_DIRECCION", ":8080"), "dónde escucha el servidor")
	f.StringVar(&c.Servicios, "servicios", valor(entorno, "REVISOR_SERVICIOS", "testdata/servicios.json"), "archivo JSON con los servicios")
	f.IntVar(&c.Paralelo, "paralelo", 5, "cuántos servicios se consultan a la vez")
	f.DurationVar(&c.Limite, "limite", 10*time.Second, "tiempo máximo para revisar todos")

	if err := f.Parse(args); err != nil {
		return Config{}, fmt.Errorf("banderas: %w", err)
	}
	if c.Paralelo <= 0 {
		return Config{}, fmt.Errorf("-paralelo tiene que ser mayor que cero, no %d", c.Paralelo)
	}
	if c.Limite <= 0 {
		return Config{}, fmt.Errorf("-limite tiene que ser mayor que cero, no %v", c.Limite)
	}
	return c, nil
}

func valor(entorno func(string) string, clave, omision string) string {
	if v := entorno(clave); v != "" {
		return v
	}
	return omision
}

// validarURL exige http o https y rechaza usuario y contraseña dentro de la URL.
//
// 🔒 Una URL como https://usuario:clave@host lleva un secreto escrito en el
// archivo, y una URL termina en bitácoras, reportes y mensajes de error. Si un
// servicio pide credenciales, van en otro lado (una variable de entorno, un
// gestor de secretos), nunca en la URL. Por eso el error NO repite la URL: solo
// dice qué está mal, para no imprimir el secreto que estamos rechazando.
func validarURL(crudo string) error {
	u, err := url.Parse(crudo)
	if err != nil {
		return errors.New("la URL no se puede leer")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("la URL tiene que empezar con http:// o https://")
	}
	if u.Host == "" {
		return errors.New("a la URL le falta el servidor")
	}
	if u.User != nil {
		return errors.New("la URL trae usuario o contraseña (usuario:clave@...); las credenciales no van en la URL")
	}
	return nil
}

// servicioJSON es la forma del archivo. Está separada de servicio.Servicio a
// propósito: el formato del archivo puede cambiar sin tocar el dominio.
type servicioJSON struct {
	Nombre  string `json:"nombre"`
	URL     string `json:"url"`
	Timeout string `json:"timeout,omitempty"` // "500ms", "2s"; vacío = el de omisión
}

// LeerServicios lee la lista de servicios y la valida: nombres únicos, URL con
// esquema y tiempos que se puedan leer. El primer error que encuentra dice en
// qué posición está, contando desde 1.
func LeerServicios(r io.Reader) ([]servicio.Servicio, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields() // un campo mal escrito es un error, no un silencio

	var crudos []servicioJSON
	if err := dec.Decode(&crudos); err != nil {
		return nil, fmt.Errorf("el archivo de servicios no es una lista JSON válida: %w", err)
	}
	if len(crudos) == 0 {
		return nil, errors.New("el archivo de servicios no declara ningún servicio")
	}

	vistos := make(map[string]int, len(crudos))
	servicios := make([]servicio.Servicio, 0, len(crudos))
	for i, c := range crudos {
		pos := i + 1
		if strings.TrimSpace(c.Nombre) == "" {
			return nil, fmt.Errorf("servicio %d: falta el nombre", pos)
		}
		if antes, repetido := vistos[c.Nombre]; repetido {
			return nil, fmt.Errorf("servicio %d: el nombre %q ya estaba en el %d", pos, c.Nombre, antes)
		}
		if err := validarURL(c.URL); err != nil {
			return nil, fmt.Errorf("servicio %d (%s): %w", pos, c.Nombre, err)
		}
		var timeout time.Duration
		if c.Timeout != "" {
			d, err := time.ParseDuration(c.Timeout)
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("servicio %d (%s): timeout %q inválido; usa 500ms, 2s o 1m", pos, c.Nombre, c.Timeout)
			}
			timeout = d
		}
		vistos[c.Nombre] = pos
		servicios = append(servicios, servicio.Servicio{Nombre: c.Nombre, URL: c.URL, Timeout: timeout})
	}
	return servicios, nil
}
