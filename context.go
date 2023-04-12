package frame

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// initContext starts a service that listens for interrupts allowing fast fails
func initContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-sigc
		cancel()
	}()
	return ctx, cancel
}
