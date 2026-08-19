package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"ruralhealth/internal/healthdb"
)

type Role string

const (
	RoleVillageDoctor Role = "village_doctor"
	RoleTownDoctor    Role = "town_doctor"
	RoleCountyExpert  Role = "county_expert"
	RoleAdmin         Role = "admin"
)

type User struct {
	ID, Username, VillageID string
	Role                    Role
}
type Service struct {
	db  *healthdb.DB
	ttl time.Duration
	now func() time.Time
}

func New(db *healthdb.DB, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	return &Service{db: db, ttl: ttl, now: time.Now}
}
func passwordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateUser(ctx context.Context, username, password string, role Role, village string) (User, error) {
	if strings.TrimSpace(username) == "" || len(password) < 8 {
		return User{}, errors.New("username and password policy not satisfied")
	}
	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.SQL().ExecContext(ctx, `INSERT INTO users(id,username,password_hash,role,village_id,created_at) VALUES(?,?,?,?,?,?)`, id, username, passwordHash(password), role, village, now)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: id, Username: username, VillageID: village, Role: role}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, User, time.Time, error) {
	var u User
	var stored string
	err := s.db.SQL().QueryRowContext(ctx, `SELECT id,username,password_hash,role,village_id FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &stored, &u.Role, &u.VillageID)
	if err != nil {
		return "", User{}, time.Time{}, fmt.Errorf("login: %w", err)
	}
	got := passwordHash(password)
	if subtle.ConstantTimeCompare([]byte(got), []byte(stored)) != 1 {
		return "", User{}, time.Time{}, errors.New("invalid credentials")
	}
	token := uuid.NewString() + uuid.NewString()
	expires := s.now().Add(s.ttl).UTC()
	_, err = s.db.SQL().ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, tokenHash(token), u.ID, expires.Format(time.RFC3339Nano), s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", User{}, time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, u, expires, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	var u User
	var expires string
	var revoked *string
	err := s.db.SQL().QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.village_id,s.expires_at,s.revoked_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash(token)).Scan(&u.ID, &u.Username, &u.Role, &u.VillageID, &expires, &revoked)
	if err != nil {
		return User{}, fmt.Errorf("authenticate: %w", err)
	}
	when, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || revoked != nil || !s.now().Before(when) {
		return User{}, healthdb.ErrRevoked
	}
	return u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.SQL().ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL`, s.now().UTC().Format(time.RFC3339Nano), tokenHash(token))
	return err
}
