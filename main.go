package main

import (
	"fmt"
	"log"

	"github.com/liangzd/hapi-lite/internal/api"
	"github.com/liangzd/hapi-lite/internal/config"
	"github.com/liangzd/hapi-lite/internal/session"
	"github.com/liangzd/hapi-lite/internal/sse"
	"github.com/liangzd/hapi-lite/internal/store"
)

func resetStaleActiveSessions(st *store.Store) {
	sessions, err := st.GetSessions()
	if err != nil {
		log.Printf("failed to load sessions for startup reset: %v", err)
		return
	}

	resetCount := 0
	for _, sess := range sessions {
		if !sess.Active {
			continue
		}
		if err := st.SetSessionActive(sess.ID, false); err != nil {
			log.Printf("failed to reset session %s active state: %v", sess.ID, err)
			continue
		}
		resetCount++
	}

	if resetCount > 0 {
		log.Printf("reset %d stale active session(s) to inactive on startup", resetCount)
	}
}

func main() {
	if err := config.Load(); err != nil {
		log.Fatal("config:", err)
	}

	st, err := store.New(config.C.DBPath)
	if err != nil {
		log.Fatal("db:", err)
	}
	defer st.Close()

	broker := sse.NewBroker()

	mgr := session.NewManager(
		func(sid string, event session.SyncEvent) {
			broker.Publish(event)
		},
		func(sid string, msg session.Message) {
			st.InsertMessage(msg)
		},
	)

	resetStaleActiveSessions(st)
	if err := st.ReindexMessageSeqs(); err != nil {
		log.Printf("failed to reindex message seq values: %v", err)
	}

	r := api.SetupRouter(st, broker, mgr)

	addr := fmt.Sprintf(":%d", config.C.Port)
	log.Printf("hapi-lite listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
