package app

import (
	"context"

	"bu1ld/internal/cache"

	"github.com/samber/oops"
)

const defaultCoordinatorAddr = "127.0.0.1:19876"

func (a *App) runServerCoordinator(ctx context.Context) error {
	addr := a.request.ListenAddr
	if addr == "" {
		addr = defaultCoordinatorAddr
	}

	server, err := cache.NewHTTPXServer(a.store, nil)
	if err != nil {
		return oops.In("bu1ld.server").
			With("addr", addr).
			Wrapf(err, "create coordinator cache server")
	}
	if err := writef(a.output, "coordinator cache server listening on %s\n", addr); err != nil {
		return err
	}

	if err := server.ListenAndServeContext(ctx, addr); err != nil && ctx.Err() == nil {
		return oops.In("bu1ld.server").
			With("addr", addr).
			Wrapf(err, "serve coordinator cache")
	}
	return nil
}
