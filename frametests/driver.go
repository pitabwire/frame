package frametests

import (
	"context"
	"net"

	"github.com/pitabwire/util"
)

func GetFreePort(ctx context.Context) (int, error) {
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	var l *net.TCPListener
	l, err = net.ListenTCP("tcp", a)
	if err != nil {
		return 0, err
	}
	defer util.CloseAndLogOnError(ctx, l)
	//nolint:errcheck //its generally expected to work
	return l.Addr().(*net.TCPAddr).Port, nil
}
