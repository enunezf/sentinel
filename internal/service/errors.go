// Package service contiene toda la lógica de negocio de Sentinel: autenticación,
// autorización, auditoría y administración de entidades (usuarios, roles, permisos,
// centros de costo). Los servicios consumen repositorios de PostgreSQL y Redis, y
// emiten eventos de auditoría de forma asíncrona a través de AuditService.
package service

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrNotFound se devuelve cuando el recurso solicitado no existe en la base de datos.
	ErrNotFound = errors.New("NOT_FOUND")

	// ErrConflict se devuelve cuando se intenta crear un recurso con un valor único
	// (username, email, code, slug) que ya existe en la base de datos. Corresponde
	// al código de error PostgreSQL 23505 (unique_violation).
	ErrConflict = errors.New("CONFLICT")
)

// isUniqueViolation informa si err es un error de violación de unicidad de PostgreSQL
// (código 23505). Se usa en los servicios para convertir errores de base de datos
// en ErrConflict antes de devolvérselos al handler.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
