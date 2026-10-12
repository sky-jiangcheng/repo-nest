// Package app is the Wails binding layer: every exported method on App is
// callable from the frontend. Methods are one-to-three-line delegations to
// internal/service — all business logic lives there and is shared with the
// CLI and MCP server.
package app

import (
	"context"

	"repotory/internal/service"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct whose public methods are exposed to the
// frontend via Wails Bind. The ctx is set during OnStartup.
type App struct {
	ctx context.Context
	svc *service.Service
}

// New creates a new App backed by the given service.
func New(svc *service.Service) *App {
	return &App{svc: svc}
}

// Service exposes the underlying service (used by main for lifecycle calls).
func (a *App) Service() *service.Service { return a.svc }

// startup is called at application startup: it wires the import-event
// forwarder (service → Wails event) and initialises the service runtime.
// Exported because Wails options require a package-level callable; guarded
// against double invocation via sync.Once in the service.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.svc.SetImportEventHandler(func(payload service.ImportEventPayload) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "import.completed", payload)
		}
	})
	a.svc.Startup()
}

// Shutdown is called when the application exits.
func (a *App) Shutdown(_ context.Context) {
	a.svc.Shutdown()
	_ = a.svc.Close()
}

// Health returns a health-check payload for the frontend.
func (a *App) Health() map[string]any { return a.svc.Health() }
