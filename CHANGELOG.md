# Changelog

## [0.8.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.7.1...v0.8.0) (2026-07-16)


### Features

* include namespace/pod in Slack message title ([62720b5](https://github.com/next-gen-infrastructure/alerting-relay/commit/62720b5870a2d38ec775c9ca305cf92ffd764fae))
* keep root message header/actions current on every alert change ([91ac487](https://github.com/next-gen-infrastructure/alerting-relay/commit/91ac4873749bb2faffde9666ff0e24cacf8ff1bc))
* send real Slack pings for the alert Team field ([6538ff4](https://github.com/next-gen-infrastructure/alerting-relay/commit/6538ff4ea953f13078b09d1713f96260747f96df))

## [0.7.1](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.7.0...v0.7.1) (2026-07-15)


### Bug Fixes

* **chart:** default image tag to appVersion, pullPolicy to Always for latest ([536965c](https://github.com/next-gen-infrastructure/alerting-relay/commit/536965cddb1eb3b0e338995bd58e63d945714b3c))

## [0.7.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.6.0...v0.7.0) (2026-07-15)


### Features

* duplicate the root message into its thread immediately ([7cd57a3](https://github.com/next-gen-infrastructure/alerting-relay/commit/7cd57a31c2d9fbec31c63ae34dcb765749aefcb7))
* group alert instances by status and add default oncall team ([a222f33](https://github.com/next-gen-infrastructure/alerting-relay/commit/a222f33719837a14408f6390c22bc1e18fe92609))

## [0.6.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.5.0...v0.6.0) (2026-07-15)


### Features

* show firing/resolved alert counts in Slack message header ([8a54210](https://github.com/next-gen-infrastructure/alerting-relay/commit/8a54210af3bb53e5e88f24c2e53adb80fb1e5973))

## [0.5.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.4.0...v0.5.0) (2026-07-15)


### Features

* add Grafana silence and dashboard buttons to Slack alerts ([6355cfd](https://github.com/next-gen-infrastructure/alerting-relay/commit/6355cfd3932047443f713cb1c574205cd2f45266))

## [0.4.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.3.0...v0.4.0) (2026-07-15)


### Features

* add default cluster routing, structured logging, and explicit pod uid/gid ([8af0a88](https://github.com/next-gen-infrastructure/alerting-relay/commit/8af0a8815d57977b908c23c885b04c7d9de31014))

## [0.3.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.2.0...v0.3.0) (2026-07-15)


### Features

* **chart:** rework alerting-relay Helm chart for production readiness ([ff0f264](https://github.com/next-gen-infrastructure/alerting-relay/commit/ff0f2646005875cb7e9af8502cd30e0e5c979dc7))

## [0.2.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.1.0...v0.2.0) (2026-07-15)


### Features

* add alerting-relay service, Helm chart, and CI/CD ([426cfaa](https://github.com/next-gen-infrastructure/alerting-relay/commit/426cfaa7b2894d5b7d60fc9301547512269ab76d))
