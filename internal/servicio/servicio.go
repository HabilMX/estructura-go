// Package servicio es el vocabulario del programa: qué es un servicio y qué es
// el estado en que lo encontramos.
//
// Es el paquete del que dependen todos y que no depende de ninguno: no abre
// conexiones, no lee archivos y no sabe de JSON. Por eso nunca puede formar un
// ciclo de importaciones, y por eso se prueba sin red y sin disco.
package servicio

import (
	"fmt"
	"time"
)

// TimeoutPorOmision es lo que se le concede a un servicio que no declara cuánto
// se le espera. Dos segundos: suficiente para una red sana y corto para que un
// servicio caído no detenga la revisión completa.
const TimeoutPorOmision = 2 * time.Second

// Servicio es algo que sabemos consultar por HTTP.
type Servicio struct {
	Nombre  string        // cómo se llama en el reporte; único en la lista
	URL     string        // dirección completa, con http:// o https://
	Timeout time.Duration // cuánto se le espera; cero significa «el de omisión»
}

// TimeoutEfectivo devuelve el tiempo límite que de verdad se aplica.
//
// Existe para que el valor cero sirva: un Servicio sin Timeout no se queda con
// un límite de cero, que significaría «no esperes nada» y fallaría siempre.
func (s Servicio) TimeoutEfectivo() time.Duration {
	if s.Timeout <= 0 {
		return TimeoutPorOmision
	}
	return s.Timeout
}

// Estado es el resultado de haber consultado un servicio: una fotografía, no un
// objeto que cambia.
//
// No lleva etiquetas json a propósito. Lo que sale por la API lo decide el
// paquete httpapi con su propio tipo: si un campo cambia de nombre aquí, el
// formato que otros programas ya leen no se rompe.
type Estado struct {
	Servicio Servicio      // a quién se consultó
	Codigo   int           // código HTTP; 0 si nunca hubo respuesta
	Duracion time.Duration // cuánto tardó la consulta
	Err      error         // por qué falló, o nil
}

// OK dice si el servicio está sano: respondió, y respondió con un 2xx.
func (e Estado) OK() bool {
	return e.Err == nil && e.Codigo >= 200 && e.Codigo <= 299
}

// Motivo explica en una línea por qué el estado no es OK. Devuelve la cadena
// vacía cuando el servicio está sano.
func (e Estado) Motivo() string {
	switch {
	case e.OK():
		return ""
	case e.Err != nil:
		return e.Err.Error()
	case e.Codigo == 0:
		return "sin respuesta"
	default:
		return fmt.Sprintf("respondió %d", e.Codigo)
	}
}
