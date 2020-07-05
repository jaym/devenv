/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	platformv1 "github.com/jaym/kube-dev-env/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type PGConfig struct {
	Host     string
	Port     uint16
	Database string
	User     string
	Password string
}

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	// PGConfig is configuration to connect to a postgres instance
	// with superuser credentials
	PGConfig PGConfig

	db *pgxpool.Pool
}

func (c *PGConfig) ToSecret(secKey client.ObjectKey) *corev1.Secret {
	data := map[string][]byte{
		"PGHOST":     []byte(c.Host),
		"PGPORT":     []byte(strconv.Itoa(int(c.Port))),
		"PGDATABASE": []byte(c.Database),
		"PGUSER":     []byte(c.User),
		"PGPASSWORD": []byte(c.Password),
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
		},
		Data: data,
		Type: "Opaque",
	}
}

// +kubebuilder:rbac:groups=platform.dev.env,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.dev.env,resources=databases/status,verbs=get;update;patch

func (r *DatabaseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("database", req.NamespacedName)

	var db platformv1.Database
	if err := r.Client.Get(ctx, req.NamespacedName, &db); err != nil {
		log.Error(err, "failed to get database")
		return ctrl.Result{}, err
	}

	var sec corev1.Secret
	secKey := objectKeyForDatabaseSecret(req.NamespacedName)
	if err := r.Client.Get(ctx, secKey, &sec); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		sec := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secKey.Name,
				Namespace: secKey.Namespace,
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
			Type: "Opaque",
		}

		if err := controllerutil.SetControllerReference(&db, &sec, r.Scheme); err != nil {
			log.Error(err, "failed to set controller")
		}

		if err := r.Client.Create(ctx, &sec); err != nil {
			log.Error(err, "failed to create secret")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	} else {
		log.Info("got secret", "foo", string(sec.Data["foo"]))
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) ensureDatabaseAndSecret(ctx context.Context, log logr.Logger, databaseObjKey client.ObjectKey) error {
	resetPassword := false
	createRole := false
	createDatabase := false
	secExists := true

	var sec corev1.Secret
	secKey := objectKeyForDatabaseSecret(databaseObjKey)
	if err := r.Client.Get(ctx, secKey, &sec); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		resetPassword = true
		secExists = false
	}

	cfg := r.pgConnConfigFor(databaseObjKey)

	var unused string
	row := r.db.QueryRow(ctx, `SELECT rolname FROM pg_catalog.pgauthid WHERE rolname=$1`, cfg.User)
	if err := row.Scan(&unused); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Error(err, "could not check if role exists", "role", cfg.User)
			return err
		}
		resetPassword = true
		createRole = true
	}

	row = r.db.QueryRow(ctx, `SELECT datname FROM pg_catalog.pg_database WHERE datname=$1`, cfg.Database)
	if err := row.Scan(&unused); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Error(err, "could not check if database exists", "database", cfg.User)
			return err
		}
		createDatabase = true
	}

	if createRole {
		if _, err := r.db.Exec(ctx, fmt.Sprintf("CREATE USER %q", cfg.User)); err != nil {
			log.Error(err, "failed to create role", "role", cfg.User)
			return err
		}
	}

	if resetPassword {
		if _, err := r.db.Exec(ctx, fmt.Sprintf(
			`ALTER USER "%s" WITH PASSWORD '%s'`, cfg.User, cfg.Password)); err != nil {

			log.Error(err, "failed to change role password", "role", cfg.User)
			return err
		}
	}

	if createDatabase {
		if _, err := r.db.Exec(ctx, fmt.Sprintf(
			`CREATE DATABASE %q OWNER %q`, cfg.Database, cfg.User)); err != nil {
			log.Error(err, "failed to create database", "role", cfg.User)
			return err
		}
	}

	if secExists {
		r.Client.Update(ctx, &sec)
	} else {
		sec := cfg.ToSecret(secKey)
		if err := r.Client.Create(ctx, sec); err != nil {
			log.Error(err, "failed to create secret")
			return err
		}
	}

	return nil
}

func (r *DatabaseReconciler) pgConnConfigFor(databaseObjKey client.ObjectKey) PGConfig {
	var cfg PGConfig
	cfg = r.PGConfig
	cfg.User = roleNameForObjectKey(databaseObjKey)
	cfg.Password = roleNameForObjectKey(databaseObjKey)
	return cfg
}

func databaseNameForObjectKey(databaseObjKey client.ObjectKey) string {
	return fmt.Sprintf("%s-%s", databaseObjKey.Namespace, databaseObjKey.Name)
}

func roleNameForObjectKey(databaseObjKey client.ObjectKey) string {
	return fmt.Sprintf("%s-%s", databaseObjKey.Namespace, databaseObjKey.Name)
}

func objectKeyForDatabaseSecret(databaseObjKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: databaseObjKey.Namespace,
		Name:      fmt.Sprintf("database-creds-%s", databaseObjKey.Name),
	}
}

func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.Database{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
