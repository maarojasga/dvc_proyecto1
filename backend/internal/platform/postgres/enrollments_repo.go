package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/enrollment"
)

type EnrollmentRepo struct{ pool *pgxpool.Pool }

func NewEnrollmentRepo(pool *pgxpool.Pool) *EnrollmentRepo { return &EnrollmentRepo{pool: pool} }

const enrollmentColumns = `id, student_id, course_id, course_version_id, status, enrolled_at, withdrawn_at, completed_at, approved_at`

func scanEnrollment(row pgx.Row) (*enrollment.Enrollment, error) {
	var e enrollment.Enrollment
	err := row.Scan(&e.ID, &e.StudentID, &e.CourseID, &e.CourseVersionID, &e.Status,
		&e.EnrolledAt, &e.WithdrawnAt, &e.CompletedAt, &e.ApprovedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &e, err
}

func (r *EnrollmentRepo) Create(ctx context.Context, e *enrollment.Enrollment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO enrollments (id, student_id, course_id, course_version_id, status, enrolled_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		e.ID, e.StudentID, e.CourseID, e.CourseVersionID, e.Status, e.EnrolledAt)
	return err
}

func (r *EnrollmentRepo) GetByStudentAndCourse(ctx context.Context, studentID, courseID uuid.UUID) (*enrollment.Enrollment, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+enrollmentColumns+` FROM enrollments WHERE student_id=$1 AND course_id=$2`, studentID, courseID)
	return scanEnrollment(row)
}

func (r *EnrollmentRepo) GetByID(ctx context.Context, id uuid.UUID) (*enrollment.Enrollment, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+enrollmentColumns+` FROM enrollments WHERE id=$1`, id)
	return scanEnrollment(row)
}

func (r *EnrollmentRepo) Update(ctx context.Context, e *enrollment.Enrollment) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE enrollments SET course_version_id=$2, status=$3, enrolled_at=$4,
			withdrawn_at=$5, completed_at=$6, approved_at=$7
		WHERE id=$1`,
		e.ID, e.CourseVersionID, e.Status, e.EnrolledAt, e.WithdrawnAt, e.CompletedAt, e.ApprovedAt)
	return err
}

func (r *EnrollmentRepo) ListByStudent(ctx context.Context, studentID uuid.UUID) ([]*enrollment.Enrollment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+enrollmentColumns+` FROM enrollments WHERE student_id=$1 ORDER BY enrolled_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*enrollment.Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
