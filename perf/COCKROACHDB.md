# CockroachDB

Perf supports using CockroachDB as a backend.

There should be a `cockroachdb/connect-*-cockroachdb.sh` script for each
instance in this directory. Run that script and you will be dropped into an SQL
shell as root to send commands to the server. You can, for example, create a new
database here:

        CREATE DATABASE androidx;

See also: https://www.cockroachlabs.com/docs/stable/authorization.html

You can get to the admin interface for each by running the associated
`cockroachdb/admin-*-cockroachdb.sh` script, which will set up a port-forward to
the admin interface and launch chrome to load the interface.

## Setup

Follow the instructions on [how to set up an instance of CockroachDB on a single
kubernetes cluster](https://www.cockroachlabs.com/docs/stable/orchestrate-cockroachdb-with-kubernetes-insecure.html#manual).

But after downloading the statefulset yaml:

    curl -O https://raw.githubusercontent.com/cockroachdb/cockroach/master/cloud/kubernetes/cockroachdb-statefulset.yaml

The file should be renamed to reflect the Perf instance that will use it, and the
names and app labels in the file should be rename to also reflect the Perf instance, for example:

    s/cockroachdb/perf-androidx-cockroachdb/g

The service name and user account are then used in the connection string in the config file, for example:

    {
      "data_store_config": {
      "datastore_type": "cockroachdb",
      "connection_string": "postgresql://root@perf-androidx-cockroachdb-public:26257/androidx?sslmode=disable",
      ...
    }

See also [configs](./configs/README.md)

## Migrations

In theory the migrations library will take care of schema migrations on startup, but
in reality, if running more than one instance, contention for the lock can arise. The
migrations can be applied from the desktop by using the migrations command line app, which
can be installed by:

    go get -tags 'cockroachdb'  github.com/golang-migrate/migrate/cmd/migrate

Now port-forwarding the database port for the cockroachdb instance, for example:

    kubectl port-forward perf-skia-cockroachdb-0 26257

Then run the migrations:

    migrate -verbose -path ./migrations/cockroachdb/ -database cockroachdb://root@localhost:26257/skia?sslmode=disable up
