package commands

import "github.com/alekzonder/tariboy/internal/stores"

// Response types of the stores.go commands. Each command declares its type
// with registry.WithResult, which derives the OpenAPI result schema from it.
// The stores package types are copied under a Store prefix, so their component
// names cannot collide with another domain's.

// StoreView is a registered Store: its name, the source it was added from and
// the local directory it is read from (store.add, store.list).
type StoreView stores.Store

// StoreImageView is one image of a Store and the state of its local build.
type StoreImageView stores.StoreImage

// StoreWorkflowView is one workflow of a Store.
type StoreWorkflowView stores.StoreWorkflow

// StoreAutoBuild is a Store's automatic refresh and build policy. An
// interval of 0 disables it.
type StoreAutoBuild stores.AutoBuild

// StoreDetail is a Store with its images, workflows and automatic build policy
// (store.show, store.refresh, store.auto).
type StoreDetail struct {
	Name      string              `json:"name"`
	Source    string              `json:"source"`
	Path      string              `json:"path"`
	Images    []StoreImageView    `json:"images"`
	Workflows []StoreWorkflowView `json:"workflows"`
	Auto      StoreAutoBuild      `json:"auto"`
}

// StoreRemoveResult is the result of store.remove.
type StoreRemoveResult struct {
	Removed bool `json:"removed"`
}

func storeDetail(d stores.Detail) StoreDetail {
	return StoreDetail{
		Name: d.Name, Source: d.Source, Path: d.Path,
		Images:    convertSlice(d.Images, func(i stores.StoreImage) StoreImageView { return StoreImageView(i) }),
		Workflows: convertSlice(d.Workflows, func(w stores.StoreWorkflow) StoreWorkflowView { return StoreWorkflowView(w) }),
		Auto:      StoreAutoBuild(d.Auto),
	}
}
