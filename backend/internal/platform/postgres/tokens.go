package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// RepositorioTokens implementa user.RepositorioTokens.
type RepositorioTokens struct{ pool *pgxpool.Pool }

// NuevoRepositorioTokens construye el repositorio.
func NuevoRepositorioTokens(pool *pgxpool.Pool) *RepositorioTokens {
	return &RepositorioTokens{pool: pool}
}

// Crear guarda el token de un solo uso.
func (r *RepositorioTokens) Crear(ctx context.Context, t user.TokenUnicoUso) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tokens_unico_uso (id, usuario_id, proposito, token_hash, creado_en, expira_en, usado_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.UsuarioID, string(t.Proposito), t.Hash, t.CreadoEn, t.ExpiraEn, t.UsadoEn)
	return err
}

// PorHash busca el token por su hash y proposito.
func (r *RepositorioTokens) PorHash(ctx context.Context, proposito user.Proposito, hash []byte) (user.TokenUnicoUso, error) {
	var t user.TokenUnicoUso
	var prop string
	err := r.pool.QueryRow(ctx, `
		SELECT id, usuario_id, proposito, token_hash, creado_en, expira_en, usado_en
		  FROM tokens_unico_uso
		 WHERE token_hash = $1 AND proposito = $2`, hash, string(proposito)).
		Scan(&t.ID, &t.UsuarioID, &prop, &t.Hash, &t.CreadoEn, &t.ExpiraEn, &t.UsadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.TokenUnicoUso{}, user.ErrNoEncontrado
	}
	if err != nil {
		return user.TokenUnicoUso{}, err
	}
	t.Proposito = user.Proposito(prop)
	return t, nil
}

// MarcarUsado consume el token.
//
// La condicion usado_en IS NULL hace que consumirlo sea atomico: si dos
// peticiones llegan con el mismo token, solo una afecta una fila y la otra
// recibe ErrNoEncontrado, que el servicio traduce a token invalido.
func (r *RepositorioTokens) MarcarUsado(ctx context.Context, id uuid.UUID, ahora time.Time) error {
	etiqueta, err := r.pool.Exec(ctx,
		`UPDATE tokens_unico_uso SET usado_en = $2 WHERE id = $1 AND usado_en IS NULL`, id, ahora)
	if err != nil {
		return err
	}
	if etiqueta.RowsAffected() == 0 {
		return user.ErrTokenInvalido
	}
	return nil
}

// InvalidarPendientes anula los tokens vigentes de ese proposito, de modo que
// emitir uno nuevo deje inservibles los anteriores.
func (r *RepositorioTokens) InvalidarPendientes(ctx context.Context, usuarioID uuid.UUID, proposito user.Proposito, ahora time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tokens_unico_uso SET usado_en = $3
		 WHERE usuario_id = $1 AND proposito = $2 AND usado_en IS NULL`,
		usuarioID, string(proposito), ahora)
	return err
}
