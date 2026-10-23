// Package revisar consulta muchos servicios a la vez.
//
// Lo que este paquete USA lo declara él mismo: la interfaz Consultor vive aquí,
// junto a Todos, que es quien la necesita. La consulta de verdad por HTTP
// (http.go) es un tipo concreto que cumple esa interfaz sin saberlo, y las
// pruebas usan un consultor falso que vive en un archivo _test.go, fuera del
// programa que se compila.
package revisar

import (
	"context"
	"sync"

	"github.com/HabilMX/estructura-go/internal/servicio"
)

// ParaleloPorOmision es cuántos servicios se consultan al mismo tiempo cuando
// nadie dice otra cosa.
const ParaleloPorOmision = 5

// Consultor es lo único que Todos necesita: alguien que consulte UN servicio y
// diga cómo lo encontró. Un método, y nada más.
//
// Nunca devuelve error: «este servicio no responde» es justo lo que el programa
// quiere reportar, así que el fracaso viaja dentro de servicio.Estado.
type Consultor interface {
	Consultar(ctx context.Context, s servicio.Servicio) servicio.Estado
}

// Todos consulta todos los servicios a la vez y devuelve sus estados en el
// MISMO orden en que llegaron.
//
// Cada goroutine escribe en su propia casilla del resultado (la i), así que
// ninguna pisa a otra y no hace falta candado. El semáforo limita cuántas
// consultas hay abiertas a la vez: con mil servicios y sin límite, el cuello de
// botella sería la máquina propia, no los servicios.
func Todos(ctx context.Context, c Consultor, servicios []servicio.Servicio, paralelo int) []servicio.Estado {
	if paralelo <= 0 {
		paralelo = ParaleloPorOmision
	}

	estados := make([]servicio.Estado, len(servicios))
	semaforo := make(chan struct{}, paralelo)

	var wg sync.WaitGroup
	for i, s := range servicios {
		wg.Go(func() {
			semaforo <- struct{}{}        // pide turno; espera si ya hay «paralelo» corriendo
			defer func() { <-semaforo }() // devuelve el turno pase lo que pase

			// Cada servicio tiene su propio límite, hijo del general: si el de
			// arriba se cancela, éste muere con él.
			propio, cancelar := context.WithTimeout(ctx, s.TimeoutEfectivo())
			defer cancelar()

			estados[i] = c.Consultar(propio, s)
		})
	}
	wg.Wait()
	return estados
}
