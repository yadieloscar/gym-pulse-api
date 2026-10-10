#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
design_file="${repo_dir}/docs/DATABASE_DESIGN.md"
migrations_dir="${repo_dir}/migrations"

if [[ ! -f "${design_file}" ]]; then
  echo "database design validation failed: docs/DATABASE_DESIGN.md is missing" >&2
  exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

sed -nE \
  's/^[[:space:]]*CREATE TABLE( IF NOT EXISTS)?[[:space:]]+([a-zA-Z0-9_.]+).*/\2/p' \
  "${migrations_dir}"/*.up.sql |
  awk '{ if (index($0, ".") == 0) print "public." $0; else print $0 }' |
  sort -u >"${work_dir}/migration-tables"

awk '
  /^## 6\. Physical schema catalog/ { in_catalog = 1; next }
  /^## 7\./ { in_catalog = 0 }
  in_catalog && /^### Table: `/ {
    table = $0
    sub(/^### Table: `/, "", table)
    sub(/`.*/, "", table)
    print table
  }
' "${design_file}" |
  sort -u >"${work_dir}/documented-tables"

missing_tables="$(comm -23 "${work_dir}/migration-tables" "${work_dir}/documented-tables")"
extra_tables="$(comm -13 "${work_dir}/migration-tables" "${work_dir}/documented-tables")"

awk '
  function qualify(name) { return index(name, ".") ? name : "public." name }
  {
    line = $0
    if (line ~ /^[[:space:]]*CREATE TABLE( IF NOT EXISTS)?[[:space:]]+[a-zA-Z0-9_.]+[[:space:]]*\(/) {
      table = line
      sub(/^[[:space:]]*CREATE TABLE( IF NOT EXISTS)?[[:space:]]+/, "", table)
      sub(/[[:space:]]*\(.*/, "", table)
      table = qualify(table)
      in_create = 1
      next
    }
    if (in_create) {
      if (line ~ /^\);/) {
        in_create = 0
        table = ""
        next
      }
      if (line ~ /^    [a-zA-Z_][a-zA-Z0-9_]*[[:space:]]+/) {
        column = line
        sub(/^    /, "", column)
        split(column, parts, /[[:space:]]+/)
        if (parts[1] !~ /^(UNIQUE|CHECK|CONSTRAINT|PRIMARY|FOREIGN)$/) {
          print table "|" parts[1]
        }
      }
      next
    }
    if (line ~ /^[[:space:]]*ALTER TABLE[[:space:]]+[a-zA-Z0-9_.]+/) {
      alter_table = line
      sub(/^[[:space:]]*ALTER TABLE[[:space:]]+/, "", alter_table)
      sub(/[[:space:];].*/, "", alter_table)
      alter_table = qualify(alter_table)
    }
    if (alter_table != "" && line ~ /ADD COLUMN[[:space:]]+[a-zA-Z_][a-zA-Z0-9_]*/) {
      column = line
      sub(/^.*ADD COLUMN[[:space:]]+/, "", column)
      sub(/[[:space:]].*/, "", column)
      print alter_table "|" column
    }
    if (alter_table != "" && line ~ /;[[:space:]]*$/) {
      alter_table = ""
    }
  }
' "${migrations_dir}"/*.up.sql |
  sort -u >"${work_dir}/migration-columns"

awk '
  /^## 6\. Physical schema catalog/ { in_catalog = 1; next }
  /^## 7\./ { in_catalog = 0 }
  in_catalog && /^### Table: `/ {
    table = $0
    sub(/^### Table: `/, "", table)
    sub(/`.*/, "", table)
    next
  }
  in_catalog && table != "" && /^\| `[^`]+` \|/ {
    column = $0
    sub(/^\| `/, "", column)
    sub(/`.*/, "", column)
    print table "|" column
  }
' "${design_file}" |
  sort -u >"${work_dir}/documented-columns"

missing_columns="$(comm -23 "${work_dir}/migration-columns" "${work_dir}/documented-columns")"
extra_columns="$(comm -13 "${work_dir}/migration-columns" "${work_dir}/documented-columns")"

sed -nE \
  's/^[[:space:]]*CREATE( UNIQUE)? INDEX[[:space:]]+([a-zA-Z0-9_]+).*/\2/p' \
  "${migrations_dir}"/*.up.sql |
  sort -u >"${work_dir}/migration-indexes"

: >"${work_dir}/missing-indexes"
while IFS= read -r index_name; do
  if ! grep -Fq "${index_name}" "${design_file}"; then
    printf '%s\n' "${index_name}" >>"${work_dir}/missing-indexes"
  fi
done <"${work_dir}/migration-indexes"

status=0
if [[ -n "${missing_tables}" ]]; then
  echo "database design validation failed: migration tables missing from the design:" >&2
  printf '%s\n' "${missing_tables}" >&2
  status=1
fi
if [[ -n "${extra_tables}" ]]; then
  echo "database design validation failed: current-table headings not created by migrations:" >&2
  printf '%s\n' "${extra_tables}" >&2
  status=1
fi
if [[ -n "${missing_columns}" ]]; then
  echo "database design validation failed: migration columns missing from the design:" >&2
  printf '%s\n' "${missing_columns}" >&2
  status=1
fi
if [[ -n "${extra_columns}" ]]; then
  echo "database design validation failed: documented columns not present after migrations:" >&2
  printf '%s\n' "${extra_columns}" >&2
  status=1
fi
if [[ -s "${work_dir}/missing-indexes" ]]; then
  echo "database design validation failed: named migration indexes missing from the design:" >&2
  sed -n '1,240p' "${work_dir}/missing-indexes" >&2
  status=1
fi

table_count="$(wc -l <"${work_dir}/migration-tables" | tr -d ' ')"
documented_column_count="$(wc -l <"${work_dir}/documented-columns" | tr -d ' ')"
index_count="$(wc -l <"${work_dir}/migration-indexes" | tr -d ' ')"
migration_column_count="$(wc -l <"${work_dir}/migration-columns" | tr -d ' ')"
# Keep the prose inventory honest without pinning the checker to a historical
# migration count. Exact set comparisons above detect missing or extra pairs.
read -r declared_table_count declared_column_count < <(
  sed -nE 's/^The current inventory contains \*\*([0-9]+) tables and ([0-9]+) supported columns\*\*.*/\1 \2/p' "${design_file}"
) || true
if [[ "${table_count}" -eq 0 || "${migration_column_count}" -eq 0 ]]; then
  echo "database design validation failed: migration inventory is empty" >&2
  status=1
fi
if [[ "${declared_table_count:-}" != "${table_count}" || "${declared_column_count:-}" != "${migration_column_count}" ]]; then
  echo "database design validation failed: declared inventory does not match ${table_count} migration tables and ${migration_column_count} columns" >&2
  status=1
fi

if [[ "${status}" -ne 0 ]]; then
  exit "${status}"
fi

echo "database design validation passed: ${table_count} tables, ${documented_column_count} columns, and ${index_count} named migration indexes documented"
