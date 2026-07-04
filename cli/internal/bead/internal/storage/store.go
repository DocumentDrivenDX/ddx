package storage

import bead "github.com/DocumentDrivenDX/ddx/internal/bead"

type Store = bead.Store
type StoreOption = bead.StoreOption

var NewStore = bead.NewStore
var NewStoreWithCollection = bead.NewStoreWithCollection
var WithCollection = bead.WithCollection

const DefaultCollection = bead.DefaultCollection

var _ Backend = (*Store)(nil)
