// Package identity owns account credentials, durable login limits and session
// revocation. Callers receive public user metadata, never password hashes.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"hwops/internal/domain"
)

const SessionLifetime = 12 * time.Hour

type Service struct {
	db    *pgxpool.Pool
	dummy []byte
}

type Session struct {
	User      domain.User `json:"user"`
	CSRFToken string      `json:"csrf_token"`
	Token     string      `json:"-"`
}

func New(db *pgxpool.Pool) (*Service, error) {
	if db == nil {
		return nil, errors.New("identity requires PostgreSQL")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), 12)
	return &Service{db: db, dummy: dummy}, err
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

func credentials(username, password string) (string, []byte, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) || len(password) < 12 || len(password) > 72 {
		return "", nil, fmt.Errorf("%w: 用户名需为 3–64 位字母、数字、点、下划线或短横线；密码需为 12–72 字节。", domain.ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return username, hash, err
}

func (s *Service) CreateUser(ctx context.Context, actor, username, password, role string, bootstrap bool) (domain.User, error) {
	username, hash, err := credentials(username, password)
	if err != nil {
		return domain.User{}, err
	}
	if role != "ADMIN" && role != "USER" {
		return domain.User{}, domain.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('hwops:accounts'))"); err != nil {
		return domain.User{}, err
	}
	if bootstrap {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists); err != nil {
			return domain.User{}, err
		}
		if exists || role != "ADMIN" {
			return domain.User{}, fmt.Errorf("%w: bootstrap is only allowed for an empty account store", domain.ErrConflict)
		}
	} else if err = requireAdmin(ctx, tx, actor); err != nil {
		return domain.User{}, err
	}
	u := domain.User{ID: rand.Text(), Username: username, Role: role, Active: true, CreatedAt: time.Now().UTC()}
	_, err = tx.Exec(ctx, "INSERT INTO users(id,username,password_hash,role,created_at) VALUES ($1,$2,$3,$4,$5)",
		u.ID, u.Username, string(hash), u.Role, u.CreatedAt)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return domain.User{}, fmt.Errorf("%w: username already exists", domain.ErrConflict)
	}
	if err != nil {
		return domain.User{}, err
	}
	return u, tx.Commit(ctx)
}

func requireAdmin(ctx context.Context, tx pgx.Tx, id string) error {
	var allowed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND active AND role='ADMIN')", id).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) List(ctx context.Context, actor, after string) ([]domain.User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = requireAdmin(ctx, tx, actor); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id,username,role,active,created_at FROM users WHERE id>$1 ORDER BY id LIMIT 50", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		var u domain.User
		if err = rows.Scan(&u.ID, &u.Username, &u.Role, &u.Active, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Update serializes administrator changes so the last active administrator
// cannot be removed. Disabling and password reset revoke every existing session.
func (s *Service) Update(ctx context.Context, actor, id string, active *bool, password *string) (domain.User, error) {
	var hash []byte
	var err error
	if password != nil {
		_, hash, err = credentials("reset", *password)
		if err != nil {
			return domain.User{}, err
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('hwops:accounts'))"); err != nil {
		return domain.User{}, err
	}
	if err = requireAdmin(ctx, tx, actor); err != nil {
		return domain.User{}, err
	}
	var u domain.User
	err = tx.QueryRow(ctx, "SELECT id,username,role,active,created_at FROM users WHERE id=$1 FOR UPDATE", id).
		Scan(&u.ID, &u.Username, &u.Role, &u.Active, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, domain.ErrNotFound
	}
	if err != nil {
		return u, err
	}
	if active != nil && !*active && u.Active && u.Role == "ADMIN" {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE active AND role='ADMIN'").Scan(&count); err != nil {
			return u, err
		}
		if count <= 1 {
			return u, fmt.Errorf("%w: cannot disable the last administrator", domain.ErrConflict)
		}
	}
	if active != nil {
		u.Active = *active
		if _, err = tx.Exec(ctx, "UPDATE users SET active=$2 WHERE id=$1", id, *active); err != nil {
			return u, err
		}
	}
	if password != nil {
		if _, err = tx.Exec(ctx, "UPDATE users SET password_hash=$2 WHERE id=$1", id, string(hash)); err != nil {
			return u, err
		}
	}
	if password != nil || !u.Active {
		if _, err = tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id=$1", id); err != nil {
			return u, err
		}
	}
	return u, tx.Commit(ctx)
}

func tokenHash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
func csrf(value string) string      { return tokenHash("hwops:csrf:" + value) }

func (s *Service) Login(ctx context.Context, username, password, address string) (Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	// Hash before persistence so failed login records retain neither names nor IPs.
	for _, limit := range []struct {
		key string
		max int
	}{{"ip:" + tokenHash(address), 30}, {"user:" + tokenHash(username), 5}} {
		var attempts int
		err := s.db.QueryRow(ctx, `INSERT INTO login_limits(key,attempts,expires_at) VALUES ($1,1,now()+interval '15 minutes')
			ON CONFLICT (key) DO UPDATE SET
			attempts=CASE WHEN login_limits.expires_at<=now() THEN 1 ELSE login_limits.attempts+1 END,
			expires_at=CASE WHEN login_limits.expires_at<=now() THEN EXCLUDED.expires_at ELSE login_limits.expires_at END
			RETURNING attempts`, limit.key).Scan(&attempts)
		if err != nil {
			return Session{}, err
		}
		if attempts > limit.max {
			return Session{}, domain.ErrRateLimited
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	var u domain.User
	var hash string
	err = tx.QueryRow(ctx, "SELECT id,username,role,active,created_at,password_hash FROM users WHERE username=$1 FOR UPDATE", username).
		Scan(&u.ID, &u.Username, &u.Role, &u.Active, &u.CreatedAt, &hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, err
	}
	if hash == "" {
		hash = string(s.dummy)
	}
	match := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if !match || !u.Active || u.ID == "" {
		return Session{}, domain.ErrUnauthorized
	}
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	// Bound live sessions per account while preserving newest logins.
	if _, err = tx.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at<=now() OR token_hash IN
		(SELECT token_hash FROM auth_sessions WHERE user_id=$1 ORDER BY created_at DESC OFFSET 19)`, u.ID); err != nil {
		return Session{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES ($1,$2,$3)",
		tokenHash(token), u.ID, time.Now().Add(SessionLifetime)); err != nil {
		return Session{}, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM login_limits WHERE key=$1 OR expires_at<=now()", "user:"+tokenHash(username)); err != nil {
		return Session{}, err
	}
	return Session{User: u, Token: token, CSRFToken: csrf(token)}, tx.Commit(ctx)
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if len(token) != 43 {
		return Session{}, domain.ErrUnauthorized
	}
	var u domain.User
	err := s.db.QueryRow(ctx, `SELECT u.id,u.username,u.role,u.active,u.created_at FROM auth_sessions a
		JOIN users u ON u.id=a.user_id WHERE a.token_hash=$1 AND a.expires_at>now() AND u.active`, tokenHash(token)).
		Scan(&u.ID, &u.Username, &u.Role, &u.Active, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, domain.ErrUnauthorized
	}
	return Session{User: u, Token: token, CSRFToken: csrf(token)}, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1", tokenHash(token))
	return err
}
