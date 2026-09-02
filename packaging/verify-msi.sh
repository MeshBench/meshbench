#!/usr/bin/env bash
# Assert that a built .msi is the installer it was meant to be.
#
#   packaging/verify-msi.sh <file.msi> <version> <upgrade-code> [file-count]
#
# Read out of the database rather than out of the WiX source, because the
# source is what somebody intended and the tables are what Windows will act on.
# Nothing here needs Windows: msiinfo reads the tables on any machine.
#
# The three things worth being sure of, in the order they hurt when wrong:
# upgrade, which installs a second copy beside the first when it is wrong; the
# version, which is what a person reads in Apps and Features without launching
# anything; and the file list, because an installer carrying meshbench.exe
# alone would install a build that cannot emulate a board and cannot say why.
set -euo pipefail

msi=${1:?the .msi to check}
version=${2:?the version it should carry}
upgrade=${3:?the upgrade code it should carry}
want_files=${4:-}

fail=0
say() { echo "verify-msi: $1"; }
bad() { echo "::error::$1" >&2; fail=1; }

# msiinfo writes the IDT form: three header rows, and DOS line endings, both of
# which would otherwise turn every comparison below into a mystery.
table() { msiinfo export "$msi" "$1" | tr -d '\r' | tail -n +4; }
prop() { table Property | awk -F'\t' -v k="$1" '$1 == k { print $2 }'; }
upper() { tr '[:lower:]' '[:upper:]'; }

# Every search below reads a here-string rather than a pipe, and that is not a
# style choice. grep -q closes its input at the first match, the writer takes a
# SIGPIPE, and pipefail then reports the whole pipeline as failed - so a check
# passes on a small table and reports the opposite on a large one, which is
# exactly what the File table did.
holds() { grep -q -- "$2" <<<"$1"; }
line() { grep -qx -- "$2" <<<"$1"; }

tables=$(msiinfo tables "$msi" | tr -d '\r')
for t in Property Directory Component File Feature Shortcut Upgrade Icon InstallExecuteSequence; do
  line "$tables" "$t" || bad "$msi has no $t table, so it is not a working installer"
done

got=$(prop ProductVersion)
[ "$got" = "$version" ] || bad "$msi says version $got, and this build is $version"
say "version $got"

# Braced in the tables, bare on the command line, and either case in either.
want="{$(echo "$upgrade" | upper)}"
got=$(prop UpgradeCode | upper)
[ "$got" = "$want" ] || bad "$msi has upgrade code $got, want $want"
say "upgrade code $got, which is fixed for the life of the product"

# The product code has to exist and has to differ from the upgrade code: equal
# to it, every version would look like the same product and none would upgrade.
got=$(prop ProductCode | upper)
case "$got" in
  "{"*"}") [ "$got" != "$want" ] || bad "$msi uses the upgrade code as its product code" ;;
  *) bad "$msi has no product code" ;;
esac
say "product code $got, derived from the version"

# The row that finds an older MeshBench and hands it to RemoveExistingProducts.
# Without it, installing 0.3.0 over 0.2.0 leaves both.
upgrades=$(table Upgrade | upper)
holds "$upgrades" "$want" ||
  bad "$msi has no Upgrade row for $want, so it would install beside an older version"
holds "$upgrades" OLDERVERSIONBEINGUPGRADED ||
  bad "$msi detects no older version to remove"
holds "$upgrades" NEWERVERSIONDETECTED ||
  bad "$msi would let an older build install over a newer one"
say "older versions are removed and a newer one is refused"

# Early, so the old product is gone before the new files are written.
holds "$(table InstallExecuteSequence)" '^RemoveExistingProducts' ||
  bad "$msi never runs RemoveExistingProducts, so an upgrade would install a second copy"
say "RemoveExistingProducts is sequenced"

dirs=$(table Directory)
holds "$dirs" '^INSTALLDIR' ||
  bad "$msi has no INSTALLDIR, so there is no location to choose"
holds "$dirs" ProgramFiles64Folder ||
  bad "$msi does not install under Program Files"
[ "$(prop ALLUSERS)" = "2" ] ||
  bad "$msi is not a per-machine-or-per-user package: ALLUSERS is '$(prop ALLUSERS)'"
# Without this, INSTALLDIR on the command line is dropped the moment the
# install elevates, and a chosen location is quietly ignored.
case "$(prop SecureCustomProperties)" in
  *INSTALLDIR*) ;;
  *) bad "$msi does not secure INSTALLDIR, so a per-machine install would ignore it" ;;
esac
say "installs under Program Files, per machine or per user, into a chosen location"

holds "$(table Shortcut)" ProgramMenuFolder || bad "$msi makes no Start menu entry"
say "Start menu entry present"

files=$(table File)
for f in meshbench.exe installed-by-msi.txt; do
  holds "$files" "$f" || bad "$msi does not carry $f"
done
n=$(grep -c . <<<"$files")
if [ -n "$want_files" ] && [ "$n" != "$want_files" ]; then
  bad "$msi carries $n files and the bundle has $want_files: the installer is not the bundle"
fi
say "$n files"

if [ "$fail" -ne 0 ]; then
  echo "verify-msi: $msi is not shippable" >&2
  exit 1
fi
echo "verify-msi: $msi is the installer it claims to be"
