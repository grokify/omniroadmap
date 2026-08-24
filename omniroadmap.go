// Package omniroadmap is the batteries-included entry point for the
// omniroadmap ecosystem: a common, tool-agnostic representation of
// roadmap/product-management data with pluggable providers.
//
// Importing this package registers every bundled provider adapter (via
// their init functions), so providers can be constructed by name:
//
//	p, err := omniroadmap.NewProvider("aha", ahaClient)
//
// Bundled providers:
//   - "aha"          — live Aha! API (github.com/grokify/aha-go/omniroadmap;
//     config: *aha.Client)
//   - "aha-studio"   — aha-studio's local SQLite cache — no Aha API traffic
//     (github.com/grokify/aha-studio/omniroadmap; config: *sync.DB)
//   - "productboard" — live ProductBoard API
//     (github.com/grokify/productboard-go/omniroadmap; config: *productboard.Client)
//   - "jpd"          — Jira Product Discovery ideas-as-issues
//     (github.com/grokify/go-atlassian/omniroadmap; config: *jira.Client)
//
// The core contract (interfaces, canonical types, registry, conformance
// tests) lives in github.com/grokify/omniroadmap-core; this package
// re-exports its public API so most consumers need only one import.
package omniroadmap

import (
	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"

	// Blank imports register the bundled provider adapters.
	_ "github.com/grokify/aha-go/omniroadmap"
	_ "github.com/grokify/aha-studio/omniroadmap"
	_ "github.com/grokify/go-atlassian/omniroadmap"
	_ "github.com/grokify/productboard-go/omniroadmap"
)

// Core interface and canonical types, re-exported from omniroadmap-core.
type (
	Provider              = provider.Provider
	Item                  = provider.Item
	ItemKind              = provider.ItemKind
	Release               = provider.Release
	Status                = provider.Status
	StatusCategory        = provider.StatusCategory
	CustomField           = provider.CustomField
	CustomFieldDefinition = provider.CustomFieldDefinition
	Person                = provider.Person
	Link                  = provider.Link
	RICE                  = provider.RICE
	Capabilities          = provider.Capabilities

	ListItemsRequest                   = provider.ListItemsRequest
	ListItemsResponse                  = provider.ListItemsResponse
	GetItemRequest                     = provider.GetItemRequest
	ListReleasesRequest                = provider.ListReleasesRequest
	ListReleasesResponse               = provider.ListReleasesResponse
	ListStatusesRequest                = provider.ListStatusesRequest
	ListStatusesResponse               = provider.ListStatusesResponse
	ListCustomFieldDefinitionsRequest  = provider.ListCustomFieldDefinitionsRequest
	ListCustomFieldDefinitionsResponse = provider.ListCustomFieldDefinitionsResponse

	APIError = omniroadmap.APIError
	Factory  = omniroadmap.Factory
)

// Item kinds.
const (
	ItemKindFeature    = provider.ItemKindFeature
	ItemKindEpic       = provider.ItemKindEpic
	ItemKindInitiative = provider.ItemKindInitiative
	ItemKindObjective  = provider.ItemKindObjective
	ItemKindKeyResult  = provider.ItemKindKeyResult
)

// Status categories.
const (
	StatusCategoryTodo       = provider.StatusCategoryTodo
	StatusCategoryInProgress = provider.StatusCategoryInProgress
	StatusCategoryDone       = provider.StatusCategoryDone
	StatusCategoryCanceled   = provider.StatusCategoryCanceled
)

// Sentinel errors.
var (
	ErrUnsupportedProvider  = omniroadmap.ErrUnsupportedProvider
	ErrProviderExists       = omniroadmap.ErrProviderExists
	ErrInvalidConfiguration = omniroadmap.ErrInvalidConfiguration
	ErrNotFound             = omniroadmap.ErrNotFound
	ErrUnsupportedOperation = omniroadmap.ErrUnsupportedOperation
)

// Registry functions.
var (
	NewProvider         = omniroadmap.NewProvider
	RegisterProvider    = omniroadmap.RegisterProvider
	RegisteredProviders = omniroadmap.RegisteredProviders
)

// Error helpers.
var (
	NewAPIError            = omniroadmap.NewAPIError
	IsNotFound             = omniroadmap.IsNotFound
	IsUnsupportedOperation = omniroadmap.IsUnsupportedOperation
)
