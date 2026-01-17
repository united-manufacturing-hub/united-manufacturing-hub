# Management Console Changelog

Release notes for the Management Console web application.

---

## January 2026

*Released: 2026-01-15*

### New Features

- **Dark Mode Toggle** - Switch between light and dark themes from settings. The interface respects your system preference by default, with manual override available.

- **Setup Assistant Wizard** - New guided workflow for creating bridges from scratch. The wizard walks you through template selection, connection configuration, and deployment.

- **Dynamic Protocol Forms** - Protocol configuration forms now show live examples and validation. Input fields display placeholder values based on the selected protocol.

- **Resizable Topic Browser** - The metadata panel in the Topic Browser can now be resized. Drag the divider to adjust the view based on your needs.

### Improvements

- **Avatar with Profile Pictures** - User avatars now display profile pictures from Auth0 when available.

- **Feature Flags System** - Expanded feature flag support for controlled rollout of new functionality.

<details>
<summary>Technical Notes</summary>

- Updated to Svelte 5 runes-based reactivity
- New mode-watcher package for system preference detection
- 71 files modified for CSS variable system

</details>

---

## December 2025

*Released: 2025-12-01*

### New Features

- **Instance Update/Install Dialog** - New unified dialog for updating instances to the latest version or installing specific releases.

- **Bridge Health Indicators** - Connection status now shows visual indicators for bridge health, including throughput metrics and latency.

### Bug Fixes

- Fixed form button alignment in location configuration forms
- Resolved sidebar collapse state persistence issue
- Fixed timezone handling in timestamp displays
