package handlers

import "net/http"

type Gateway struct {
	orderProxy http.Handler
	readProxy  http.Handler
	frontend   http.Handler
}

func New(orderProxy http.Handler, readProxy http.Handler, frontend http.Handler) *Gateway {
	return &Gateway{orderProxy: orderProxy, readProxy: readProxy, frontend: frontend}
}

func (g *Gateway) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", g.health)
	mux.Handle("GET /", g.frontend)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", g.frontend))
	mux.Handle("POST /orders", g.orderProxy)
	mux.Handle("GET /orders", g.readProxy)
	mux.Handle("POST /orders/{order_id}/cancel", g.orderProxy)
	mux.Handle("GET /orders/{order_id}", g.readProxy)
	mux.Handle("GET /orders/{order_id}/history", g.readProxy)
}

func (g *Gateway) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
