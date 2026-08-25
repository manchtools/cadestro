package handler

import sdk "github.com/manchtools/cadestro/contract"

var (
	_ sdk.StreamHandler    = (*Handler)(nil)
	_ sdk.LuksHandler      = (*Handler)(nil)
	_ sdk.LogQueryHandler  = (*Handler)(nil)
	_ sdk.InventoryHandler = (*Handler)(nil)
)
