# CHANGELOG
Changelog for ecszap

## unreleased

## 1.0.1
* Updated zap to v1.21.1 [pull#42](https://github.com/elastic/ecs-logging-go-zap/pull/42)

## 1.0.0

### Enhancement
* Add `ecszap.WrapCoreOption` for convenience [pull#22](https://github.com/elastic/ecs-logging-go-zap/pull/22)

### Bug Fixes
* Change `stacktrace` to `stack_trace` in output and in json and yaml config option for `EncoderConfig.EnableStacktrace` [pull#21](https://github.com/elastic/ecs-logging-go-zap/pull/21)
* Do not allow configuration of `@timestamp`, instead always set format to ISO8601 [pull#23](https://github.com/elastic/ecs-logging-go-zap/pull/23)

## 0.3.0

### Enhancement
* Update ECS version to 1.6.0 [pull#17](https://github.com/elastic/ecs-logging-go-zap/pull/17)

## 0.2.0

### Enhancement
* Add `ecszap.ECSCompatibleEncoderConfig` for making existing encoder config ECS conformant [pull#12](https://github.com/elastic/ecs-logging-go-zap/pull/12)
* Add method `ToZapCoreEncoderConfig` to `ecszap.EncoderConfig` for advanced use cases [pull#12](https://github.com/elastic/ecs-logging-go-zap/pull/12)

### Bug Fixes
* Use `zapcore.ISO8601TimeEncoder` as default instead of `ecszap.EpochMicrosTimeEncoder` [pull#12](https://github.com/elastic/ecs-logging-go-zap/pull/12)

### Breaking Change
* remove `ecszap.NewJSONEncoder` [pull#12](https://github.com/elastic/ecs-logging-go-zap/pull/12)

## 0.1.0
Initial Pre-Release supporting [MVP](https://github.com/elastic/ecs-logging/tree/main/spec#minimum-viable-product) for ECS conformant logging
