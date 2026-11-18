// Command fibonacci-service serves the first n Fibonacci numbers over HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mattiaslndstrm/fibonacci-service/internal/httpapi"
)

const (
	readTimeout     = 5 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second
	shutdownTimeout = 5 * time.Second
	maxHeaderBytes  = 8192

	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fibonacci-service", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addr := flags.String("addr", ":8080", "address to listen on")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}

		return exitUsage
	}

	if flags.NArg() > 0 {
		if _, err := fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0)); err == nil {
			flags.Usage()
		}

		return exitUsage
	}

	logger := slog.New(slog.NewJSONHandler(stdout, nil))

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Stops catching signals after the first signal. A second signal kills the process.
	context.AfterFunc(ctx, stop)

	if err := serve(ctx, *addr, logger); err != nil {
		logger.Error("server failed", "error", err)

		return exitFailure
	}

	return 0
}

// serve answers requests on addr. When ctx is cancelled, it gives the requests in flight shutdownTimeout to finish.
func serve(ctx context.Context, addr string, logger *slog.Logger) error {
	var listenConfig net.ListenConfig

	// Listen ignores the cancellation of ctx. The select below handles it as a shutdown.
	listener, err := listenConfig.Listen(context.WithoutCancel(ctx), "tcp", addr)
	if err != nil {
		return err
	}

	logger.Info("listening", "addr", listener.Addr().String())

	server := &http.Server{
		Handler:        httpapi.NewHandler(logger),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		MaxHeaderBytes: maxHeaderBytes,
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	failed := make(chan error, 1)

	go func() { failed <- server.Serve(listener) }()

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		closeErr := server.Close()

		return fmt.Errorf("shutdown: %w", errors.Join(err, closeErr))
	}

	return nil
}
