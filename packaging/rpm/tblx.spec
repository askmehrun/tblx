# RPM spec for tblx (Tablix) — Fedora / RHEL / CentOS Stream.
#
# Build:
#   cd packaging/rpm
#   rpmbuild -bb tblx.spec
# Install:
#   sudo dnf install ~/rpmbuild/RPMS/$(uname -m)/tblx-1.0.0-1.$(uname -m).rpm

%global debug_package %{nil}

Name:           tblx
Version:        1.0.0
Release:        1%{?dist}
Summary:        Tablix — binary columnar file format (TBLX) and terminal CLI

License:        MIT
URL:            https://github.com/askmehrun/tblx
Source0:        %{url}/archive/refs/tags/v%{version}/%{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.26
ExclusiveArch:  x86_64 aarch64

%description
Tablix (tblx) is a CLI + TUI for the TBLX columnar binary format: it
converts CSV to TBL through an interactive type wizard, browses .tblx
files in a full-screen terminal UI, prints metadata, and exports back to
CSV. One contiguous, directly seekable block per column, three types
(int64, float64, string), little-endian throughout. The format core lives
in the libtblx module and uses only the Go standard library.

%prep
%setup -q -n %{name}-%{version}

%build
export CGO_ENABLED=0
export GOFLAGS="-mod=mod"
# The repository carries a local `replace` directive for development
# against a sibling libtblx checkout. Package builds must not use it —
# strip it and let Go fetch the published libtblx module instead.
go mod tidy
go build -trimpath -buildmode pie -ldflags "-s -w -B 0x$(head -c20 /dev/urandom | od -An -tx1 | tr -d ' \n')" -o %{name} ./cmd/%{name}

%install
install -D -p -m 0755 %{name} %{buildroot}%{_bindir}/%{name}
install -D -p -m 0644 README.md %{buildroot}%{_docdir}/%{name}/README.md

%check
go vet ./...

%files
%{_bindir}/%{name}
%{_docdir}/%{name}/README.md

%changelog
* Sun Jan 04 2026 Tablix maintainers <tblx@example.com> - 1.0.0-1
- Initial package: TBLX format (via libtblx), tblx CLI with
  import/view/info/export, interactive Bubble Tea UIs.
