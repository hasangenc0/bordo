import { WebTracerProvider } from '@opentelemetry/sdk-trace-web'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http'
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-web'
import { Resource } from '@opentelemetry/resources'
import { SEMRESATTRS_SERVICE_NAME } from '@opentelemetry/semantic-conventions'

export function initTracing(serviceName: string): void {
  const endpoint = import.meta.env.VITE_OTEL_ENDPOINT
  if (!endpoint) return

  const provider = new WebTracerProvider({
    resource: new Resource({ [SEMRESATTRS_SERVICE_NAME]: serviceName }),
  })

  provider.addSpanProcessor(
    new BatchSpanProcessor(new OTLPTraceExporter({ url: `${endpoint}/v1/traces` })),
  )

  provider.register()
}
