package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"ruralhealth/internal/auth"
	"ruralhealth/internal/health"
	"ruralhealth/internal/healthdb"
)

type app struct {
	db     *healthdb.DB
	auth   *auth.Service
	health *health.Service
}

func main() {
	path := os.Getenv("RURAL_HEALTH_DB")
	if path == "" {
		path = "./data/health.db"
	}
	db, err := healthdb.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	a := auth.New(db, 8*time.Hour)
	h := health.New(db, a)
	mux := http.NewServeMux()
	x := &app{db: db, auth: a, health: h}
	mux.HandleFunc("/healthz", x.healthz)
	mux.HandleFunc("/api/v1/auth/users", x.createUser)
	mux.HandleFunc("/api/v1/auth/login", x.login)
	mux.HandleFunc("/api/v1/auth/logout", x.logout)
	mux.HandleFunc("/api/v1/villagers", x.createVillager)
	mux.HandleFunc("/api/v1/visits", x.recordVisit)
	mux.HandleFunc("/api/v1/referrals", x.createReferral)
	mux.HandleFunc("/api/v1/referrals/accept", x.acceptReferral)
	log.Println("healthapi listening on :52700")
	log.Fatal(http.ListenAndServe(":52700", withRequestID(mux)))
}
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		next.ServeHTTP(w, r)
	})
}
func (a *app) healthz(w http.ResponseWriter, r *http.Request) {
	if err := a.db.Ping(r.Context()); err != nil {
		jsonError(w, 503, err)
		return
	}
	jsonWrite(w, 200, map[string]string{"status": "ready"})
}
func (a *app) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username, Password string
		Role               auth.Role
		VillageID          string
	}
	if decode(w, r, &in) != nil {
		return
	}
	u, err := a.auth.CreateUser(r.Context(), in.Username, in.Password, in.Role, in.VillageID)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	jsonWrite(w, 201, u)
}
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if decode(w, r, &in) != nil {
		return
	}
	token, u, expires, err := a.auth.Login(r.Context(), in.Username, in.Password)
	if err != nil {
		jsonError(w, 401, err)
		return
	}
	jsonWrite(w, 200, map[string]any{"token": token, "expires_at": expires, "user": u})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		jsonError(w, 401, errors.New("missing token"))
		return
	}
	if err := a.auth.Logout(r.Context(), token); err != nil {
		jsonError(w, 500, err)
		return
	}
	jsonWrite(w, 204, nil)
}
func (a *app) actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, err := a.auth.Authenticate(r.Context(), bearer(r))
	if err != nil {
		jsonError(w, 401, err)
		return auth.User{}, false
	}
	return u, true
}
func (a *app) createVillager(w http.ResponseWriter, r *http.Request) {
	u, ok := a.actor(w, r)
	if !ok {
		return
	}
	var in struct{ Name, Phone, BirthDate string }
	if decode(w, r, &in) != nil {
		return
	}
	v, err := a.health.CreateVillager(r.Context(), u, in.Name, in.Phone, in.BirthDate)
	if err != nil {
		jsonError(w, 403, err)
		return
	}
	jsonWrite(w, 201, v)
}
func (a *app) recordVisit(w http.ResponseWriter, r *http.Request) {
	u, ok := a.actor(w, r)
	if !ok {
		return
	}
	var in struct {
		VillagerID, Kind, ScheduledAt, Note, IdempotencyKey string
		Systolic, Diastolic                                 *int
		Glucose                                             *float64
	}
	if decode(w, r, &in) != nil {
		return
	}
	v, err := a.health.RecordVisit(r.Context(), u, in.VillagerID, in.Kind, in.ScheduledAt, in.Note, in.IdempotencyKey, in.Systolic, in.Diastolic, in.Glucose)
	if err != nil {
		jsonError(w, 409, err)
		return
	}
	jsonWrite(w, 201, v)
}
func (a *app) createReferral(w http.ResponseWriter, r *http.Request) {
	u, ok := a.actor(w, r)
	if !ok {
		return
	}
	var in struct{ VillagerID, ToDoctor, Reason string }
	if decode(w, r, &in) != nil {
		return
	}
	v, err := a.health.CreateReferral(r.Context(), u, in.VillagerID, in.ToDoctor, in.Reason)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	jsonWrite(w, 201, v)
}
func (a *app) acceptReferral(w http.ResponseWriter, r *http.Request) {
	u, ok := a.actor(w, r)
	if !ok {
		return
	}
	var in struct {
		ID      string
		Version int
	}
	if decode(w, r, &in) != nil {
		return
	}
	if err := a.health.AcceptReferral(r.Context(), u, in.ID, in.Version); err != nil {
		jsonError(w, 409, err)
		return
	}
	jsonWrite(w, 204, nil)
}
func bearer(r *http.Request) string {
	p := strings.Fields(r.Header.Get("Authorization"))
	if len(p) == 2 && strings.EqualFold(p[0], "bearer") {
		return p[1]
	}
	return ""
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		jsonError(w, 400, err)
		return err
	}
	return nil
}
func jsonWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func jsonError(w http.ResponseWriter, status int, err error) {
	jsonWrite(w, status, map[string]string{"error": err.Error()})
}

var _ = context.Background
