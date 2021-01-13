package ginopentracing

import (
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

// All funcs are DEPRECATED (they all use a deprecated jaeger client func)

// Config - the open tracing config singleton
var Config jaegercfg.Configuration

// InitProduction - DEPRECATED (uses deprecated jaeger client func) init a production tracer environment
// example: Create the default tracer and schedule its closing when main returns.
//    func main() {
//	tracing.InitProduction("jaegeragent.svc.cluster.local:6831")
//	tracer, closer, _ := tracing.Config.New("passport-gigya-user-access") // the service name is the param to New()
//	defer closer.Close()
//	opentracing.SetGlobalTracer(tracer)
//
func InitProduction(sampleProbability float64, tracingAgentHostPort []byte) {
	Config = jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeProbabilistic,
			Param: sampleProbability,
		},
		Reporter: reporterConfig(tracingAgentHostPort),
	}
}

// InitDevelopment - DEPRECATED (uses deprecated jaeger client func) init a production tracer environment
// example: Create the default tracer and schedule its closing when main returns.
//    func main() {
//  tracing.InitDevelopment() # defaults to "localhost:6831" for tracing agent
//	tracer, closer, _ := tracing.Config.New("passport-gigya-user-access") // the service name is the param to New()
//	defer closer.Close()
//	opentracing.SetGlobalTracer(tracer)
//
func InitDevelopment(tracingAgentHostPort []byte) {
	Config = jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: reporterConfig(tracingAgentHostPort),
	}
}

// InitMacDocker - DEPRECATED (uses deprecated jaeger client func) init a production tracer environment
// example: Create the default tracer and schedule its closing when main returns.
//    func main() {
//  tracing.InitMacDocker() # defaults to "host.docker.internal:6831 for tracing agent
//	tracer, closer, _ := tracing.Config.New("passport-gigya-user-access") // the service name is the param to New()
//	defer closer.Close()
//	opentracing.SetGlobalTracer(tracer)
//
func InitMacDocker(tracingAgentHostPort []byte) {
	var reporter *jaegercfg.ReporterConfig
	if tracingAgentHostPort != nil {
		reporter = reporterConfig(tracingAgentHostPort)
	} else {
		reporter = reporterConfig([]byte("host.docker.internal:6831"))
	}
	Config = jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: reporter,
	}
}

func reporterConfig(hostPort []byte) *jaegercfg.ReporterConfig {
	agentPort := "localhost:6831"
	if hostPort != nil {
		agentPort = string(hostPort)
	}
	return &jaegercfg.ReporterConfig{
		LogSpans:           true,
		LocalAgentHostPort: agentPort,
	}
}
