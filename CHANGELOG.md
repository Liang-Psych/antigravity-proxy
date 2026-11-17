# Changelog

## [0.3.1](https://github.com/Liang-Psych/antigravity-proxy/compare/v0.3.0...v0.3.1) (2026-11-17)


### Bug Fixes

* **proxy:** stop blaming Google for client cancellations and refresh catalogs after 404 ([d31a5f7](https://github.com/Liang-Psych/antigravity-proxy/commit/d31a5f7cc5b8bfc7c5f2ec8f2f8032efe0c0418d))
* update default App client identity to 2.32.1 ([#2](https://github.com/Liang-Psych/antigravity-proxy/issues/2)) ([97a91fc](https://github.com/Liang-Psych/antigravity-proxy/commit/97a91fc2362d163fd8c8ac35827b0efb60f211e7))

## [0.3.0](https://github.com/Liang-Psych/antigravity-proxy/compare/v0.2.0...v0.3.0) (2026-11-17)


### Features

* configure client identity and preserve sanitized diagnostics ([66afb0f](https://github.com/Liang-Psych/antigravity-proxy/commit/66afb0f7a3ae0d3c50dbfdaaeae2cf967f56a5f1))
* inspect model catalogs across configured accounts ([bb97c49](https://github.com/Liang-Psych/antigravity-proxy/commit/bb97c4943e3d461b9e62f10382e5431d2a61d91c))
* log bounded diagnostics for failed generation streams ([8b53a10](https://github.com/Liang-Psych/antigravity-proxy/commit/8b53a10063bd85de7432aafcde6270e57a045578))
* **proxy:** queue generation bursts and bound capacity per account ([5cbd80a](https://github.com/Liang-Psych/antigravity-proxy/commit/5cbd80aa1d3bc46582605302dd30df4d44283bac))


### Bug Fixes

* **ci:** use Go 1.27.4 for patched standard library ([e7d93fc](https://github.com/Liang-Psych/antigravity-proxy/commit/e7d93fc3235b09eb0e85bb8ac18a36584ef6e0f3))
* **config:** update advertised Antigravity client version ([9860435](https://github.com/Liang-Psych/antigravity-proxy/commit/98604353aee1d9d1c1abaec1a416ecc0796213d3))
* discover and call Gemini 4 with the CLI client family ([df114dd](https://github.com/Liang-Psych/antigravity-proxy/commit/df114dde6c9ec4e9844c8a81231985868299caf8))
* **oauth:** compare token expiry by wall clock across system sleep ([7385180](https://github.com/Liang-Psych/antigravity-proxy/commit/73851808765cbbf4083422900a3f114972648538))
* **proxy:** clarify unsupported interactions and stream failures ([a670912](https://github.com/Liang-Psych/antigravity-proxy/commit/a670912a9cf53d5ca2053dfccdbbb8c7de405de6))
* **proxy:** recover once from rejected OAuth tokens ([bd42e76](https://github.com/Liang-Psych/antigravity-proxy/commit/bd42e762fa8c886f9c109c6015c7ddbbc11880d7))
* **release:** let Release Please bump the executable version ([eb9b1ef](https://github.com/Liang-Psych/antigravity-proxy/commit/eb9b1ef7f9b6df881247ba8b0101f93cf1468c1e))
* update default CLI client identity to 1.5.10 ([6e490d5](https://github.com/Liang-Psych/antigravity-proxy/commit/6e490d5b86f077ebe659b3b113880c748eeb0223))

## [0.2.0](https://github.com/terastal-hq/antigravity-proxy/compare/v0.1.0...v0.2.0) (2026-10-16)


### Features

* add OAuth session management, login account flows, runtime proxy, and status monitoring improvements ([a3c9611](https://github.com/terastal-hq/antigravity-proxy/commit/a3c96111ae981662e66f21c5fe3aa0829942f4c2))
* **ci:** unify CI/CD pipelines, code quality, and security workflows ([78940da](https://github.com/terastal-hq/antigravity-proxy/commit/78940daefb485a5228b22df4ddbef51aa124585e))
* implement multi-account system with Admin API and account key routing ([373e7e9](https://github.com/terastal-hq/antigravity-proxy/commit/373e7e914c738f91e94c0943e7a28191f7f0e113))
* **quota:** synthesize 5-hour window from weekly quota for weekly-only tiers ([fcd18a5](https://github.com/terastal-hq/antigravity-proxy/commit/fcd18a511493b82f5ad693a2af3446375c40aa21))
* scan binary for oauth strings ([7317e82](https://github.com/terastal-hq/antigravity-proxy/commit/7317e821929c26390d897852e122f0c3d8d75704))


### Bug Fixes

* **ci:** update security workflow Go version to 1.27.2 to resolve stdlib CVEs ([62c2c59](https://github.com/terastal-hq/antigravity-proxy/commit/62c2c593bbd60477d47d5160acab2f12a2dc2d29))
* **ci:** update to Go 1.27.2 for stdlib CVEs, fix actionlint warnings and platform test ([f8b9ddd](https://github.com/terastal-hq/antigravity-proxy/commit/f8b9dddf63ff9e40cf5d0c3b828bc691b3c7a16c))
* harden config and startup boundaries ([5e004c7](https://github.com/terastal-hq/antigravity-proxy/commit/5e004c7c7a0898a7d71380a00bd6d92292b3dbce))
* preserve query separators in JSON login URL responses ([0c84d40](https://github.com/terastal-hq/antigravity-proxy/commit/0c84d4018f56bf63e435930b98d86df4fcae7d05))
* preserve query separators in JSON login URL responses ([0360273](https://github.com/terastal-hq/antigravity-proxy/commit/036027313a11f60c0ae7b7536e1ebb75ecc06abd))
* **proxy:** accept per-account keys when global API_KEY is set ([ad27933](https://github.com/terastal-hq/antigravity-proxy/commit/ad27933a793a450a549fd824bf2a791b9cfe95f4)), closes [#7](https://github.com/terastal-hq/antigravity-proxy/issues/7)
