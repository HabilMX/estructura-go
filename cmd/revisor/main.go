// El programa revisor: un servidor HTTP que, cuando se le pide, consulta a la
// vez una lista de servicios y dice cuáles responden.
//
// main es DELGADO a propósito: lee la configuración, arma las piezas de
// internal/ y arranca. Si algo de aquí merece una prueba, ya no pertenece aquí.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HabilMX/estructura-go/internal/config"
	"github.com/HabilMX/estructura-go/internal/httpapi"
	"github.com/HabilMX/estructura-go/internal/revisar"
)

func main() {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	if err := ejecutar(ctx, os.Args[1:], os.Getenv, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "revisor:", err)
		os.Exit(1)
	}
}

// ejecutar hace el trabajo y devuelve un error en vez de llamar a os.Exit, para
// que una prueba pueda llamarlo.
func ejecutar(ctx context.Context, args []string, entorno func(string) string, bitacora io.Writer) error {
	log := slog.New(slog.NewTextHandler(bitacora, nil))

	cfg, err := config.Desde(args, entorno)
	if err != nil {
		return err
	}
	archivo, err := os.Open(cfg.Servicios)
	if err != nil {
		return fmt.Errorf("abriendo la lista de servicios: %w", err)
	}
	servicios, err := config.LeerServicios(archivo)
	archivo.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", cfg.Servicios, err)
	}

	srv := &http.Server{
		Addr: cfg.Direccion,
		Handler: httpapi.Nuevo(httpapi.Opciones{
			Servicios: servicios,
			Consultor: revisar.NuevoHTTP(cfg.Limite),
			Paralelo:  cfg.Paralelo,
			Limite:    cfg.Limite,
		}),
		ReadHeaderTimeout: 5 * time.Second, // sin esto, un cliente lento ocupa una conexión para siempre
	}

	escucha, err := net.Listen("tcp", cfg.Direccion)
	if err != nil {
		return fmt.Errorf("no puedo escuchar en %s: %w", cfg.Direccion, err)
	}
	log.Info("revisor escuchando", "direccion", escucha.Addr().String(), "servicios", len(servicios))

	errServidor := make(chan error, 1)
	go func() { errServidor <- srv.Serve(escucha) }()

	select {
	case err := <-errServidor:
		return err
	case <-ctx.Done():
		log.Info("apagando: termino las peticiones en curso")
		apagado, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		if err := srv.Shutdown(apagado); err != nil {
			return err
		}
		if err := <-errServidor; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
