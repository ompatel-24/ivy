#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

version=$1
release_dir=${DIST_DIR:-dist}
checksum_file="ivy_${version}_checksums.txt"
formula_file="${release_dir}/homebrew/Formula/ivy.rb"
platforms=(
  darwin_amd64
  darwin_arm64
  linux_amd64
  linux_arm64
)

if [[ ! -f "${release_dir}/${checksum_file}" ]]; then
  echo "missing checksum manifest: ${release_dir}/${checksum_file}" >&2
  exit 1
fi

if [[ ! -f "$formula_file" ]]; then
  echo "missing generated Homebrew formula: $formula_file" >&2
  exit 1
fi

expected_manifest=$(mktemp)
actual_manifest=$(mktemp)
expected_artifacts=$(mktemp)
actual_artifacts=$(mktemp)
expected_contents=$(mktemp)
actual_contents=$(mktemp)
extract_dir=$(mktemp -d)
trap 'rm -f "$expected_manifest" "$actual_manifest" "$expected_artifacts" "$actual_artifacts" "$expected_contents" "$actual_contents"; rm -rf "$extract_dir"' EXIT

for platform in "${platforms[@]}"; do
  archive="ivy_${version}_${platform}.tar.gz"
  archive_path="${release_dir}/${archive}"

  if [[ ! -f "$archive_path" ]]; then
    echo "missing release archive: $archive_path" >&2
    exit 1
  fi

  if ! grep -Fq "$archive" "$formula_file"; then
    echo "Homebrew formula does not reference $archive" >&2
    exit 1
  fi

  printf '%s\n' "$archive" >>"$expected_manifest"
  printf '%s\n' "$archive" >>"$expected_artifacts"
  printf '%s\n' LICENSE README.md ivy | LC_ALL=C sort >"$expected_contents"
  tar -tzf "$archive_path" | sed 's#^\./##' | LC_ALL=C sort >"$actual_contents"
  if ! cmp -s "$expected_contents" "$actual_contents"; then
    echo "unexpected contents in $archive:" >&2
    diff -u "$expected_contents" "$actual_contents" >&2 || true
    exit 1
  fi
done

if ! grep -Fq 'class Ivy < Formula' "$formula_file" ||
  ! grep -Fq 'assert_match version.to_s, shell_output("#{bin}/ivy version")' "$formula_file"; then
  echo "generated Homebrew formula is missing Ivy's class or version test" >&2
  exit 1
fi

printf '%s\n' "$checksum_file" >>"$expected_artifacts"
find "$release_dir" -maxdepth 1 -type f \( -name 'ivy_*.tar.gz' -o -name 'ivy_*_checksums.txt' \) \
  -exec basename {} \; | LC_ALL=C sort >"$actual_artifacts"
LC_ALL=C sort -o "$expected_artifacts" "$expected_artifacts"
if ! cmp -s "$expected_artifacts" "$actual_artifacts"; then
  echo "release directory does not contain the exact public artifact set:" >&2
  diff -u "$expected_artifacts" "$actual_artifacts" >&2 || true
  exit 1
fi

awk '{print $2}' "${release_dir}/${checksum_file}" | sed 's#^\*##' | LC_ALL=C sort >"$actual_manifest"
LC_ALL=C sort -o "$expected_manifest" "$expected_manifest"
if ! cmp -s "$expected_manifest" "$actual_manifest"; then
  echo "checksum manifest does not name exactly the four release archives:" >&2
  diff -u "$expected_manifest" "$actual_manifest" >&2 || true
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  (cd "$release_dir" && shasum -a 256 -c "$checksum_file")
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$release_dir" && sha256sum -c "$checksum_file")
else
  echo "neither shasum nor sha256sum is available" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) native_os=darwin ;;
  Linux) native_os=linux ;;
  *)
    echo "skipping native execution on unsupported host $(uname -s)" >&2
    exit 0
    ;;
esac

case "$(uname -m)" in
  arm64 | aarch64) native_arch=arm64 ;;
  x86_64 | amd64) native_arch=amd64 ;;
  *)
    echo "skipping native execution on unsupported architecture $(uname -m)" >&2
    exit 0
    ;;
esac

native_archive="${release_dir}/ivy_${version}_${native_os}_${native_arch}.tar.gz"
tar -xzf "$native_archive" -C "$extract_dir"
actual_version=$("${extract_dir}/ivy" version)
expected_version="ivy ${version}"
if [[ "$actual_version" != "$expected_version" ]]; then
  printf 'native archive version = %q, want %q\n' "$actual_version" "$expected_version" >&2
  exit 1
fi

echo "verified Ivy ${version} release artifacts"
