package api

import (
	"github.com/liangzd/hapi-lite/internal/session"
	"github.com/liangzd/hapi-lite/internal/sse"
	"github.com/liangzd/hapi-lite/internal/store"
)

type BaseHandler struct {
	Store  *store.Store
	Broker *sse.Broker
	Mgr    *session.Manager
}
