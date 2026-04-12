package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/sfzman/Narratio/backend/internal/model"
	"github.com/sfzman/Narratio/backend/internal/store"
)

var (
	_ store.JobStore      = (*Store)(nil)
	_ store.TaskStore     = (*Store)(nil)
	_ store.WorkflowStore = (*Store)(nil)
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) CreateJob(ctx context.Context, job *model.Job) error {
	return s.createJob(ctx, s.db, job)
}

func (s *Store) ListJobs(ctx context.Context) ([]model.Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		id,
		public_id,
		token,
		status,
		progress,
		spec_json,
		warnings_json,
		error_json,
		result_json,
		created_at,
		updated_at
	FROM jobs
	ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]model.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job rows: %w", err)
	}

	return jobs, nil
}

func (s *Store) InitializeJob(ctx context.Context, job *model.Job, tasks []model.Task) ([]model.Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin initialize job tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.createJob(ctx, tx, job); err != nil {
		return nil, err
	}

	for i := range tasks {
		tasks[i].JobID = job.ID
	}

	createdTasks, err := s.createTasks(ctx, tx, tasks)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit initialize job tx: %w", err)
	}

	return createdTasks, nil
}

func (s *Store) DeleteJobWorkflow(ctx context.Context, jobID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete job workflow tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("delete tasks by job %d: %w", jobID, err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, jobID)
	if err != nil {
		return fmt.Errorf("delete job %d: %w", jobID, err)
	}
	if err := expectRowsAffected(res, store.ErrJobNotFound); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete job workflow tx: %w", err)
	}

	return nil
}

func (s *Store) createJob(ctx context.Context, exec execContexter, job *model.Job) error {
	specJSON, err := json.Marshal(job.Spec)
	if err != nil {
		return fmt.Errorf("marshal job spec: %w", err)
	}

	warningsJSON, err := json.Marshal(job.Warnings)
	if err != nil {
		return fmt.Errorf("marshal job warnings: %w", err)
	}

	errorJSON, err := marshalNullable(job.Error)
	if err != nil {
		return fmt.Errorf("marshal job error: %w", err)
	}

	resultJSON, err := marshalNullable(job.Result)
	if err != nil {
		return fmt.Errorf("marshal job result: %w", err)
	}

	res, err := exec.ExecContext(
		ctx,
		`INSERT INTO jobs (
			public_id,
			token,
			status,
			progress,
			spec_json,
			warnings_json,
			error_json,
			result_json,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.PublicID,
		job.Token,
		job.Status,
		job.Progress,
		string(specJSON),
		string(warningsJSON),
		errorJSON,
		resultJSON,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	jobID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("read job last insert id: %w", err)
	}

	job.ID = jobID
	return nil
}

func (s *Store) GetJob(ctx context.Context, id int64) (model.Job, error) {
	return s.getJob(ctx, `SELECT
		id,
		public_id,
		token,
		status,
		progress,
		spec_json,
		warnings_json,
		error_json,
		result_json,
		created_at,
		updated_at
	FROM jobs
	WHERE id = ?`, id)
}

func (s *Store) GetJobByPublicID(ctx context.Context, publicID string) (model.Job, error) {
	return s.getJob(ctx, `SELECT
		id,
		public_id,
		token,
		status,
		progress,
		spec_json,
		warnings_json,
		error_json,
		result_json,
		created_at,
		updated_at
	FROM jobs
	WHERE public_id = ?`, publicID)
}

func (s *Store) UpdateJob(ctx context.Context, job model.Job) error {
	specJSON, err := json.Marshal(job.Spec)
	if err != nil {
		return fmt.Errorf("marshal job spec: %w", err)
	}

	warningsJSON, err := json.Marshal(job.Warnings)
	if err != nil {
		return fmt.Errorf("marshal job warnings: %w", err)
	}

	errorJSON, err := marshalNullable(job.Error)
	if err != nil {
		return fmt.Errorf("marshal job error: %w", err)
	}

	resultJSON, err := marshalNullable(job.Result)
	if err != nil {
		return fmt.Errorf("marshal job result: %w", err)
	}

	res, err := s.db.ExecContext(
		ctx,
		`UPDATE jobs
		SET public_id = ?,
			token = ?,
			status = ?,
			progress = ?,
			spec_json = ?,
			warnings_json = ?,
			error_json = ?,
			result_json = ?,
			created_at = ?,
			updated_at = ?
		WHERE id = ?`,
		job.PublicID,
		job.Token,
		job.Status,
		job.Progress,
		string(specJSON),
		string(warningsJSON),
		errorJSON,
		resultJSON,
		job.CreatedAt,
		job.UpdatedAt,
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job %d: %w", job.ID, err)
	}

	if err := expectRowsAffected(res, store.ErrJobNotFound); err != nil {
		return err
	}

	return nil
}

func (s *Store) DeleteJob(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete job %d: %w", id, err)
	}

	if err := expectRowsAffected(res, store.ErrJobNotFound); err != nil {
		return err
	}

	return nil
}

func (s *Store) CreateTasks(ctx context.Context, tasks []model.Task) ([]model.Task, error) {
	return s.createTasks(ctx, s.db, tasks)
}

func (s *Store) createTasks(ctx context.Context, exec beginPreparer, tasks []model.Task) ([]model.Task, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	return s.insertTasks(ctx, exec, tasks)
}

func (s *Store) insertTasks(ctx context.Context, exec preparerContexter, tasks []model.Task) ([]model.Task, error) {
	stmt, err := exec.PrepareContext(
		ctx,
		`INSERT INTO tasks (
			job_id,
			task_key,
			type,
			status,
			resource_key,
			depends_on_json,
			attempt,
			max_attempts,
			payload_json,
			output_ref_json,
			error_json,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return nil, fmt.Errorf("prepare create tasks statement: %w", err)
	}
	defer stmt.Close()

	created := make([]model.Task, 0, len(tasks))
	for _, task := range tasks {
		dependsJSON, err := json.Marshal(task.DependsOn)
		if err != nil {
			return nil, fmt.Errorf("marshal task depends_on: %w", err)
		}

		payloadJSON, err := json.Marshal(task.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal task payload: %w", err)
		}

		outputJSON, err := json.Marshal(task.OutputRef)
		if err != nil {
			return nil, fmt.Errorf("marshal task output_ref: %w", err)
		}

		errorJSON, err := marshalNullable(task.Error)
		if err != nil {
			return nil, fmt.Errorf("marshal task error: %w", err)
		}

		res, err := stmt.ExecContext(
			ctx,
			task.JobID,
			task.Key,
			task.Type,
			task.Status,
			task.ResourceKey,
			string(dependsJSON),
			task.Attempt,
			task.MaxAttempts,
			string(payloadJSON),
			string(outputJSON),
			errorJSON,
			task.CreatedAt,
			task.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert task %q: %w", task.Key, err)
		}

		taskID, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read task last insert id: %w", err)
		}

		task.ID = taskID
		created = append(created, task)
	}

	return created, nil
}

type execContexter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type preparerContexter interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

type beginPreparer interface {
	preparerContexter
}

func (s *Store) GetTask(ctx context.Context, id int64) (model.Task, error) {
	return s.getTask(ctx, `SELECT
		id,
		job_id,
		task_key,
		type,
		status,
		resource_key,
		depends_on_json,
		attempt,
		max_attempts,
		payload_json,
		output_ref_json,
		error_json,
		created_at,
		updated_at
	FROM tasks
	WHERE id = ?`, id)
}

func (s *Store) ListTasksByJob(ctx context.Context, jobID int64) ([]model.Task, error) {
	return s.listTasks(
		ctx,
		`SELECT
			id,
			job_id,
			task_key,
			type,
			status,
			resource_key,
			depends_on_json,
			attempt,
			max_attempts,
			payload_json,
			output_ref_json,
			error_json,
			created_at,
			updated_at
		FROM tasks
		WHERE job_id = ?
		ORDER BY id ASC`,
		jobID,
	)
}

func (s *Store) ListTasksByJobPublicID(ctx context.Context, publicID string) ([]model.Task, error) {
	return s.listTasks(
		ctx,
		`SELECT
			t.id,
			t.job_id,
			t.task_key,
			t.type,
			t.status,
			t.resource_key,
			t.depends_on_json,
			t.attempt,
			t.max_attempts,
			t.payload_json,
			t.output_ref_json,
			t.error_json,
			t.created_at,
			t.updated_at
		FROM tasks t
		INNER JOIN jobs j ON j.id = t.job_id
		WHERE j.public_id = ?
		ORDER BY t.id ASC`,
		publicID,
	)
}

func (s *Store) UpdateTask(ctx context.Context, task model.Task) error {
	dependsJSON, err := json.Marshal(task.DependsOn)
	if err != nil {
		return fmt.Errorf("marshal task depends_on: %w", err)
	}

	payloadJSON, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("marshal task payload: %w", err)
	}

	outputJSON, err := json.Marshal(task.OutputRef)
	if err != nil {
		return fmt.Errorf("marshal task output_ref: %w", err)
	}

	errorJSON, err := marshalNullable(task.Error)
	if err != nil {
		return fmt.Errorf("marshal task error: %w", err)
	}

	res, err := s.db.ExecContext(
		ctx,
		`UPDATE tasks
		SET job_id = ?,
			task_key = ?,
			type = ?,
			status = ?,
			resource_key = ?,
			depends_on_json = ?,
			attempt = ?,
			max_attempts = ?,
			payload_json = ?,
			output_ref_json = ?,
			error_json = ?,
			created_at = ?,
			updated_at = ?
		WHERE id = ?`,
		task.JobID,
		task.Key,
		task.Type,
		task.Status,
		task.ResourceKey,
		string(dependsJSON),
		task.Attempt,
		task.MaxAttempts,
		string(payloadJSON),
		string(outputJSON),
		errorJSON,
		task.CreatedAt,
		task.UpdatedAt,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task %d: %w", task.ID, err)
	}

	if err := expectRowsAffected(res, store.ErrTaskNotFound); err != nil {
		return err
	}

	return nil
}

func (s *Store) getJob(ctx context.Context, query string, arg any) (model.Job, error) {
	row := s.db.QueryRowContext(ctx, query, arg)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return model.Job{}, store.ErrJobNotFound
	}
	if err != nil {
		return model.Job{}, fmt.Errorf("query job: %w", err)
	}

	return job, nil
}

func (s *Store) getTask(ctx context.Context, query string, arg any) (model.Task, error) {
	row := s.db.QueryRowContext(ctx, query, arg)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return model.Task{}, store.ErrTaskNotFound
	}
	if err != nil {
		return model.Task{}, fmt.Errorf("query task: %w", err)
	}

	return task, nil
}

func (s *Store) listTasks(ctx context.Context, query string, arg any) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("query task list: %w", err)
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task rows: %w", err)
	}

	return tasks, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (model.Job, error) {
	var (
		job          model.Job
		specJSON     string
		warningsJSON string
		errorJSON    sql.NullString
		resultJSON   sql.NullString
	)

	if err := s.Scan(
		&job.ID,
		&job.PublicID,
		&job.Token,
		&job.Status,
		&job.Progress,
		&specJSON,
		&warningsJSON,
		&errorJSON,
		&resultJSON,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		return model.Job{}, err
	}

	if err := json.Unmarshal([]byte(specJSON), &job.Spec); err != nil {
		return model.Job{}, fmt.Errorf("unmarshal job spec: %w", err)
	}

	if err := json.Unmarshal([]byte(warningsJSON), &job.Warnings); err != nil {
		return model.Job{}, fmt.Errorf("unmarshal job warnings: %w", err)
	}

	if err := unmarshalNullable(errorJSON, &job.Error); err != nil {
		return model.Job{}, fmt.Errorf("unmarshal job error: %w", err)
	}

	if err := unmarshalNullable(resultJSON, &job.Result); err != nil {
		return model.Job{}, fmt.Errorf("unmarshal job result: %w", err)
	}

	return job, nil
}

func scanTask(s scanner) (model.Task, error) {
	var (
		task        model.Task
		dependsJSON string
		payloadJSON string
		outputJSON  string
		errorJSON   sql.NullString
	)

	if err := s.Scan(
		&task.ID,
		&task.JobID,
		&task.Key,
		&task.Type,
		&task.Status,
		&task.ResourceKey,
		&dependsJSON,
		&task.Attempt,
		&task.MaxAttempts,
		&payloadJSON,
		&outputJSON,
		&errorJSON,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return model.Task{}, err
	}

	if err := json.Unmarshal([]byte(dependsJSON), &task.DependsOn); err != nil {
		return model.Task{}, fmt.Errorf("unmarshal task depends_on: %w", err)
	}

	if err := json.Unmarshal([]byte(payloadJSON), &task.Payload); err != nil {
		return model.Task{}, fmt.Errorf("unmarshal task payload: %w", err)
	}

	if err := json.Unmarshal([]byte(outputJSON), &task.OutputRef); err != nil {
		return model.Task{}, fmt.Errorf("unmarshal task output_ref: %w", err)
	}

	if err := unmarshalNullable(errorJSON, &task.Error); err != nil {
		return model.Task{}, fmt.Errorf("unmarshal task error: %w", err)
	}

	return task, nil
}

func marshalNullable(v any) (any, error) {
	if v == nil {
		return nil, nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return string(data), nil
}

func unmarshalNullable[T any](src sql.NullString, dst **T) error {
	if !src.Valid || src.String == "" {
		*dst = nil
		return nil
	}

	var value T
	if err := json.Unmarshal([]byte(src.String), &value); err != nil {
		return err
	}

	*dst = &value
	return nil
}

func expectRowsAffected(res sql.Result, notFound error) error {
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected: %w", err)
	}

	if rows == 0 {
		return notFound
	}

	return nil
}
