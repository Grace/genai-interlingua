// Drives the Vercel AI SDK through one chat-with-tool-call.
//
// experimental_telemetry is what makes the SDK emit spans at all; without it a
// generateText call is invisible. recordInputs/recordOutputs default on, and are
// left on because the prompt and completion attributes are half of what the
// dialect parser exists to read.

import { openai, createOpenAI } from '@ai-sdk/openai'
import { generateText, tool } from 'ai'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { NodeTracerProvider, SimpleSpanProcessor } from '@opentelemetry/sdk-trace-node'
import { z } from 'zod'

const provider = new NodeTracerProvider({
  resource: resourceFromAttributes({ 'service.name': 'capture' }),
  spanProcessors: [
    new SimpleSpanProcessor(new OTLPTraceExporter({ url: `${process.env.SINK}/v1/traces` })),
  ],
})
provider.register()

const model = createOpenAI({ apiKey: 'mock', baseURL: `${process.env.MOCK}/v1` })

await generateText({
  // v5's openai provider defaults to the Responses API. .chat() selects the
  // chat completions endpoint, which is what the other dialects here call and
  // what the mock implements.
  model: model.chat('gpt-4o-mini'),
  system: 'You are a support agent. Use the tools you are given.',
  prompt: 'Where is order A-1187?',
  temperature: 0.7,
  topP: 0.95,
  maxOutputTokens: 1024,
  tools: {
    lookup_order: tool({
      description: 'Look up an order by its identifier.',
      inputSchema: z.object({ order_id: z.string() }),
      execute: async ({ order_id }) => ({ order_id, status: 'in transit' }),
    }),
  },
  experimental_telemetry: { isEnabled: true, functionId: 'support_triage' },
})

await provider.forceFlush()
await provider.shutdown()
console.log('vercel: done')
