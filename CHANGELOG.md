# Changelog

All notable changes to this project are documented in this file.

This repository is a **fork** of [BlackMesaLTD/terraform-provider-checkmk](https://github.com/BlackMesaLTD/terraform-provider-checkmk)
(MPL-2.0, see `LICENSE`). It is maintained and published under the `mcorralb/checkmk`
namespace on the Terraform Registry.

## [0.0.8] - 2026-10-01

### Added

- **`checkmk_ruleset_order` resource**: declares and enforces the exact order of
  the rules within a (ruleset, folder) pair. CheckMK evaluates first-match
  rulesets top-down, so this resource makes rule precedence reproducible and
  **detects and corrects any reordering done outside Terraform** (e.g. in the
  CheckMK UI). It is the building block for adopting `checkgroup_parameters:*`
  rulesets while keeping their current precedence.
  - `rules` (list of `api_id`) is read from CheckMK on refresh, so an external
    reorder shows up as a diff on `plan` and is fixed on `apply` via the
    CheckMK move action.
  - Deleting the resource is a no-op: it never reorders or deletes the rules.
- **`checkmk_rule.folder_index` (read-only)**: exposes the current position of a
  rule within its ruleset and folder, as reported by CheckMK.
- **Ordering docs**: `docs/resources/ruleset_order.md`, example in
  `examples/resources/checkmk_ruleset_order/`, `folder_index` documented on
  `checkmk_rule`, and a "Rule ordering" note in the README.
- **`LICENSE`**: MPL-2.0, identical to the upstream repository.

### Changed

- `checkmk_rule`: the internal `position` attribute (never released) was removed;
  rule ordering is now managed exclusively by `checkmk_ruleset_order`.

### Fixed

- `checkmk_rule`: restored the `resp.State.Set` call at the end of `Create`
  (regression while removing the position logic) that caused
  "Missing Resource State After Create".

## [0.0.7] - 2026-09-27

First release published under `mcorralb/checkmk` on the Terraform Registry.

### Fixed

- **`value_raw` semantic equality**: avoid eternal diffs for rules whose
  `value_raw` is re-serialized by CheckMK (Python literal whitespace).
- **Import/Read of rules**: do not read `properties` from a null prior state,
  which enables `terraform import` for rules.

## Upstream history

Earlier changes belong to the upstream repository
[BlackMesaLTD/terraform-provider-checkmk](https://github.com/BlackMesaLTD/terraform-provider-checkmk).

<!--
Format: keep-a-changelog (https://keepachangelog.com/en/1.1.0/)
Unreleased section is replaced by the version number when a release is cut.
-->