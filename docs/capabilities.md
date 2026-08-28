# Capabilities

Cadestro supports three action types.

## Package

Installs a named package, optionally at a requested version, or removes it.
The agent detects apt, dnf/dnf5, pacman, or zypper and checks installed state
before changing it.

## Update

Refreshes package metadata, checks whether updates are available, and performs
the native package manager's full-system update.

## Shell

Runs a bounded, non-interactive script as root. An optional detection script
makes remediation idempotent: exit zero means the desired state is already
present. A compliance shell action contains detection only and never
remediates.
