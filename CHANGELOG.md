# Changelog

## [0.15.2](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.15.1...v0.15.2) (2026-11-06)


### Bug Fixes

* **argocd:** more argocd statuses support ([e8b02de](https://github.com/next-gen-infrastructure/alerting-relay/commit/e8b02dee284d92e1ac36071aa0b03482c9bf31e3))

## [0.15.1](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.15.0...v0.15.1) (2026-11-06)


### Bug Fixes

* **argocd:** batch Slack updates and cap message size ([454674f](https://github.com/next-gen-infrastructure/alerting-relay/commit/454674f54613f1de031f296beccf90b7df077ac9))
* **argocd:** batch Slack updates and cap message size ([d9ebd74](https://github.com/next-gen-infrastructure/alerting-relay/commit/d9ebd74d1ff6cb223227aa5000bc7019b4b0b7ec))

## [0.15.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.14.0...v0.15.0) (2026-11-06)


### Features

* route warning alerts to alerting channel and log webhook requests ([13f2b8f](https://github.com/next-gen-infrastructure/alerting-relay/commit/13f2b8fe970748989e70fe3447301120c29f5e07))
* route warning alerts to alerting channel and log webhook requests ([3c583b9](https://github.com/next-gen-infrastructure/alerting-relay/commit/3c583b985dda02bf60eba0e05656f27d5b49e94a))

## [0.14.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.13.0...v0.14.0) (2026-11-06)


### Features

* **argocd:** aggregate app statuses per revision ([2c51f29](https://github.com/next-gen-infrastructure/alerting-relay/commit/2c51f2952857193162d2c9ca4fdaa59519e7d61a))
* **argocd:** aggregate app statuses per revision ([10d782b](https://github.com/next-gen-infrastructure/alerting-relay/commit/10d782b7ff93df9cfc104571f11bfd7a2304ad97))

## [0.13.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.12.0...v0.13.0) (2026-11-05)


### Features

* relay Argo CD notifications to Slack ([80503cd](https://github.com/next-gen-infrastructure/alerting-relay/commit/80503cdaa92fe5bf16265be5abbe8e91e72db572))
* relay Argo CD notifications to Slack ([31126a1](https://github.com/next-gen-infrastructure/alerting-relay/commit/31126a1020af7c6224c120eafe76ca07ee668f9b))

## [0.12.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.11.0...v0.12.0) (2026-11-04)


### Features

* post inhibited alerts in thread and use workload in header ([125e894](https://github.com/next-gen-infrastructure/alerting-relay/commit/125e89435bbf0e8b7aaff458f9d9432ebc720f8a))
* post inhibited alerts in thread and use workload in header ([b2655c5](https://github.com/next-gen-infrastructure/alerting-relay/commit/b2655c5b50bb19202b56100521da8709bb2109ee))

## [0.11.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.10.0...v0.11.0) (2026-10-17)


### Features

* **metrics:** expose prometheus metrics on /metrics and add PodMonitor ([f1352f7](https://github.com/next-gen-infrastructure/alerting-relay/commit/f1352f7d9ebb6b97f83b9622bc46897be4fb565c))


### Bug Fixes

* gitignore ([826a04d](https://github.com/next-gen-infrastructure/alerting-relay/commit/826a04d72cc3d3548530fe324baa7e6237f3be79))

## [0.10.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.9.2...v0.10.0) (2026-08-07)


### Features

* **slack:** fall back to app label for header scope suffix ([012b420](https://github.com/next-gen-infrastructure/alerting-relay/commit/012b42069b7feec389a61589b8ac33e7bf5edbde))
* **slack:** fall back to app label for header scope suffix ([b9d31f6](https://github.com/next-gen-infrastructure/alerting-relay/commit/b9d31f63c1053d72cd9fbc75c2fd47c9a03f8f9b))

## [0.9.2](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.9.1...v0.9.2) (2026-08-01)


### Bug Fixes

* pin SQLite connection pool to one connection ([896289e](https://github.com/next-gen-infrastructure/alerting-relay/commit/896289e7bce0b4d693b6d88dac6d7053c1dd6554))
* pin SQLite connection pool to one connection ([bb90266](https://github.com/next-gen-infrastructure/alerting-relay/commit/bb902667f192e8fc6ed93a85401a8bd76cc142ae))

## [0.9.1](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.9.0...v0.9.1) (2026-08-01)


### Bug Fixes

* skip Slack channel list lookup when SLACK_CHANNELS is already IDs ([0dab69e](https://github.com/next-gen-infrastructure/alerting-relay/commit/0dab69eef56a9c4520952a880acabe1b464ebf04))

## [0.9.0](https://github.com/next-gen-infrastructure/alerting-relay/compare/v0.8.0...v0.9.0) (2026-07-29)


### Features

* support SQLite as an alternative to Postgres ([87978fd](https://github.com/next-gen-infrastructure/alerting-relay/commit/87978fd039d84ca6b08c1b063cbee1f2e79f80c9))


### Bug Fixes

* start a new Slack thread when a resolved alert group fires again ([a2ec71e](https://github.com/next-gen-infrastructure/alerting-relay/commit/a2ec71ea614b1c0743b27e475a111c80a482af4a))

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
