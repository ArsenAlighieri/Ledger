#!/bin/sh
set -e

# Ensure data and backup directories exist and have proper ownership
mkdir -p /app/data /app/data/backups
chown -R ledger:ledger /app/data

# Execute the application as the unprivileged ledger user
exec su-exec ledger /app/ledger "$@"
