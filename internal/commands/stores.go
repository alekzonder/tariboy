package commands

import (
	"errors"
	"net/http"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
)

func storeCatalog(c *registry.Ctx) *stores.Catalog { return stores.New(c.Store, c.BaseDir) }

func storeError(err error) error {
	switch {
	case errors.Is(err, stores.ErrInvalid), errors.Is(err, stores.ErrUnsafe):
		return api.UserError{Code: "bad_store", Msg: err.Error(), Status: http.StatusBadRequest}
	case errors.Is(err, stores.ErrExists):
		return api.UserError{Code: "store_exists", Msg: err.Error(), Status: http.StatusConflict}
	case errors.Is(err, stores.ErrNotFound):
		return api.UserError{Code: "store_not_found", Msg: err.Error(), Status: http.StatusNotFound}
	default:
		return err
	}
}

func storeAdd() registry.Command {
	return registry.Command{Path: "store.add", Summary: "Register a local or Git Store", Args: []registry.Arg{
		{Name: "name", Type: registry.String, Required: true, Help: "Store name"},
		{Name: "source", Type: registry.String, Required: true, Help: "absolute local directory or Git source"},
	}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		store, err := storeCatalog(c).Add(registry.RequestContext(p), str(p, "name"), str(p, "source"))
		return store, storeError(err)
	}}
}

func storeList() registry.Command {
	return registry.Command{Path: "store.list", Summary: "List registered Stores", HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/stores"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		list, err := storeCatalog(c).List(registry.RequestContext(p))
		return list, storeError(err)
	}}
}

func storeShow() registry.Command {
	return registry.Command{Path: "store.show", Summary: "Show a Store and its images", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/stores/{name}"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		detail, err := storeCatalog(c).Detail(registry.RequestContext(p), str(p, "name"))
		return detail, storeError(err)
	}}
}

func storeRefresh() registry.Command {
	return registry.Command{Path: "store.refresh", Summary: "Refresh a Store", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores/{name}/refresh"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		detail, err := storeCatalog(c).Refresh(registry.RequestContext(p), str(p, "name"))
		return detail, storeError(err)
	}}
}

func storeRemove() registry.Command {
	return registry.Command{Path: "store.remove", Summary: "Remove a Store registration", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/stores/{name}"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		if err := storeCatalog(c).Remove(registry.RequestContext(p), str(p, "name")); err != nil {
			return nil, storeError(err)
		}
		return map[string]any{"removed": true}, nil
	}}
}
