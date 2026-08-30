package plugins

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
)

// API is the plugin-facing daemon surface: channel-source publish + subscription
// read, authenticated by a plugin-token header and scoped to the plugin's
// declared channels (spec §7.2/§13).
type API struct {
	tokens *TokenRegistry
	bus    *bus.Bus
	log    *slog.Logger
	audit  func(plugin, action, detail string)
}

func NewAPI(tokens *TokenRegistry, b *bus.Bus, log *slog.Logger, audit func(plugin, action, detail string)) *API {
	if log == nil {
		log = slog.Default()
	}
	if audit == nil {
		audit = func(string, string, string) {}
	}
	return &API{tokens: tokens, bus: b, log: log, audit: audit}
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/plugin/publish":
		a.publish(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/plugin/subscriptions":
		a.subscriptions(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/plugin/watches":
		a.watches(w, r)
	default:
		api.WriteErr(w, http.StatusNotFound, "not_found", "unknown plugin route "+r.Method+" "+r.URL.Path)
	}
}

// token extracts the plugin-token from Authorization: Bearer or X-Plugin-Token.
func token(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return strings.TrimSpace(r.Header.Get("X-Plugin-Token"))
}

func (a *API) auth(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id, ok := a.tokens.Resolve(token(r))
	if !ok {
		api.WriteErr(w, http.StatusUnauthorized, "unauthorized", "invalid plugin token")
		return Identity{}, false
	}
	return id, true
}

func (a *API) publish(w http.ResponseWriter, r *http.Request) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	var body struct {
		Channel              string         `json:"channel"`
		Type                 string         `json:"type"`
		Subject              map[string]any `json:"subject"`
		Text                 string         `json:"text"`
		Data                 map[string]any `json:"data"`
		IdempotencyKey       string         `json:"idempotency_key"`
		RequireDeliveryAgent string         `json:"require_delivery_agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteErr(w, http.StatusBadRequest, "bad_body", "invalid JSON body")
		return
	}
	if body.Channel == "" {
		api.WriteErr(w, http.StatusBadRequest, "missing_channel", "channel is required")
		return
	}
	if len(body.IdempotencyKey) > 256 {
		api.WriteErr(w, http.StatusBadRequest, "bad_idempotency_key", "idempotency_key is too long")
		return
	}
	if !id.CanPublish(body.Channel) {
		api.WriteErr(w, http.StatusForbidden, "forbidden_channel",
			"plugin "+id.Name+" may not publish to "+body.Channel)
		return
	}
	message := bus.Message{
		IdempotencyKey: pluginIdempotencyKey(id.Name, body.IdempotencyKey),
		Channel:        body.Channel, Type: body.Type, Subject: body.Subject, Text: body.Text, Data: body.Data,
		Source: "plugin:" + id.Name, ProducedByPlugin: id.Name,
	}
	var msg bus.Message
	var err error
	if body.RequireDeliveryAgent != "" {
		msg, err = a.bus.PublishRequiringDelivery(message, body.RequireDeliveryAgent)
	} else {
		msg, err = a.bus.Publish(message)
	}
	if err != nil {
		if errors.Is(err, bus.ErrRequiredDelivery) {
			api.WriteErr(w, http.StatusConflict, "delivery_required", err.Error())
			return
		}
		api.WriteErr(w, http.StatusInternalServerError, "publish_failed", err.Error())
		return
	}
	deliveredAgents, err := a.bus.DeliveryAgents(msg.ID)
	if err != nil {
		api.WriteErr(w, http.StatusInternalServerError, "delivery_lookup_failed", err.Error())
		return
	}
	// Seed-on-publish: a channel-sink plugin declares its drain surface as globs
	// (e.g. chat:*) that the exact-match bus can never deliver to. Every concrete
	// chat surfaces here first — as inbound the plugin itself publishes — so when
	// the target falls within the plugin's declared sink globs, idempotently
	// register a CONCRETE subscription for it. A later reply from another source
	// (agent) to that same channel then fans out to this sink and reaches
	// /deliver. Seeding AFTER Publish deliberately leaves the just-published
	// inbound without a delivery row for this new subscription (no echo of the
	// seeding message); drainOnce's echo-suppression covers any subsequent inbound.
	// Idempotent: migration 0020 UNIQUE(agent,channel,matcher,type_filter) makes a
	// repeat subscribe a no-op.
	if id.MatchesSink(body.Channel) {
		if _, serr := a.bus.Subscribe("plugin:"+id.Name, body.Channel, bus.Matcher{}, nil); serr != nil {
			a.log.Warn("sink seed subscribe failed", "plugin", id.Name, "channel", body.Channel, "err", serr)
		}
	}
	a.audit(id.Name, "publish", body.Channel)
	api.WriteOK(w, map[string]any{"id": msg.ID, "channel": msg.Channel, "delivered_agents": deliveredAgents})
}

func pluginIdempotencyKey(plugin, key string) string {
	if key == "" {
		return ""
	}
	return "plugin:" + plugin + ":" + key
}

func (a *API) subscriptions(w http.ResponseWriter, r *http.Request) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	chans := id.Subscribe
	if chans == nil {
		chans = []string{}
	}
	api.WriteOK(w, map[string]any{"channels": chans})
}

// watches is the pull path (spec §6.2): it returns the full current watch list
// for every channel this plugin provides, so a plugin that (re)started and lost
// its in-memory state can reconcile from scratch. Same structure the daemon
// pushes per-channel on subscribe/unsubscribe. Replaces the misleading static
// /api/plugin/subscriptions stub (kept alive for now).
func (a *API) watches(w http.ResponseWriter, r *http.Request) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	channels := make([]ChannelWatchesDTO, 0, len(id.Provide))
	for _, ch := range id.Provide {
		ws, err := a.bus.WatchList(ch)
		if err != nil {
			api.WriteErr(w, http.StatusInternalServerError, "watch_list_failed", err.Error())
			return
		}
		channels = append(channels, ChannelWatchesDTO{Channel: ch, Watches: watchDTOs(ws)})
	}
	api.WriteOK(w, map[string]any{"channels": channels})
}
