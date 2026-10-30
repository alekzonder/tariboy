package commands

import (
	"errors"
	"net/http"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/onboarding"
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
	case errors.Is(err, stores.ErrRefresh):
		return api.UserError{Code: "store_refresh_failed", Msg: err.Error(), Status: http.StatusConflict}
	default:
		return err
	}
}

func storeAdd() registry.Command {
	return registry.WithResult[StoreView](registry.Command{Path: "store.add", Summary: "Register a local or Git Store", Args: []registry.Arg{
		{Name: "name", Type: registry.String, Required: true, Help: "Store name"},
		{Name: "source", Type: registry.String, Required: true, Help: "absolute local directory or Git source"},
	}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		store, err := storeCatalog(c).Add(p.Ctx, p.String("name"), p.String("source"))
		if err != nil {
			return nil, storeError(err)
		}
		return StoreView(store), nil
	}})
}

func storeList() registry.Command {
	return registry.WithResult[[]StoreView](registry.Command{Path: "store.list", Summary: "List registered Stores", HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/stores"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		list, err := storeCatalog(c).List(p.Ctx)
		if err != nil {
			return nil, storeError(err)
		}
		return convertSlice(list, func(s stores.Store) StoreView { return StoreView(s) }), nil
	}})
}

func storeShow() registry.Command {
	return registry.WithResult[StoreDetail](registry.Command{Path: "store.show", Summary: "Show a Store and its images", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/stores/{name}"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		detail, err := storeCatalog(c).Detail(p.Ctx, p.String("name"))
		if err != nil {
			return nil, storeError(err)
		}
		return storeDetail(detail), nil
	}})
}

func storeRefresh() registry.Command {
	return registry.WithResult[StoreDetail](registry.Command{Path: "store.refresh", Summary: "Refresh a Store", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores/{name}/refresh"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		detail, err := storeCatalog(c).Refresh(p.Ctx, p.String("name"))
		if err != nil {
			return nil, storeError(err)
		}
		return storeDetail(detail), nil
	}})
}

func storeAuto() registry.Command {
	return registry.WithResult[StoreDetail](registry.Command{Path: "store.auto", Summary: "Set a Store's automatic refresh and build policy", Args: []registry.Arg{
		{Name: "name", Type: registry.String, Required: true, Help: "Store name"},
		{Name: "interval", Flag: "interval", Type: registry.Int, Help: "refresh interval in whole minutes; 0 disables automatic builds"},
		{Name: "image", Flag: "image", Type: registry.String, Repeatable: true, Help: "Store image selected for automatic builds"},
	}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores/{name}/auto"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		detail, err := storeCatalog(c).SetAuto(p.Ctx, p.String("name"), p.IntOr("interval", 0), p.Strings("image"))
		if err != nil {
			return nil, storeError(err)
		}
		return storeDetail(detail), nil
	}})
}

func storeRemove() registry.Command {
	return registry.WithResult[StoreRemoveResult](registry.Command{Path: "store.remove", Summary: "Remove a Store registration", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/stores/{name}"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		catalog, name := storeCatalog(c), p.String("name")
		removed := stores.Store{Name: name}
		if list, err := catalog.List(p.Ctx); err == nil {
			for _, s := range list {
				if s.Name == name {
					removed = s
				}
			}
		}
		if err := catalog.Remove(p.Ctx, name); err != nil {
			return nil, storeError(err)
		}
		// The daemon never registers a removed default Store again by itself.
		if err := onboarding.MarkRemoved(c.Store, removed, onboardingSource(c)); err != nil {
			return nil, err
		}
		return StoreRemoveResult{Removed: true}, nil
	}})
}
