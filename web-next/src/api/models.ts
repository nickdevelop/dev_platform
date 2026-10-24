// Model management API calls. DTOs mirror docs/api/openapi.yaml model schemas
// and the server's modelconfig DTOs; the API key is write-only and never
// returned (services expose hasKey and an opaque secretRef).
import { get, mutate } from './client'

export interface ModelDto {
  id: number
  serviceId: number
  modelId: string
  name: string
  supportsText: boolean
  supportsImage: boolean
  supportsTools: boolean
  contextTokens: number
  outputTokens: number
  enabled: boolean
  usable: boolean
}

export interface ServiceDto {
  id: number
  name: string
  baseUrl: string
  hasKey: boolean
  secretRef: string
  enabled: boolean
  models: ModelDto[]
}

export interface ServiceList {
  services: ServiceDto[]
}

export interface ServiceInput {
  name: string
  baseUrl: string
  apiKey: string
  enabled: boolean
}

export interface ModelInput {
  modelId: string
  name: string
  supportsText: boolean
  supportsImage: boolean
  supportsTools: boolean
  contextTokens: number
  outputTokens: number
  enabled: boolean
}

export interface ProbeInput {
  baseUrl: string
  apiKey?: string
  secretRef?: string
}

export interface TestInput extends ProbeInput {
  modelId: string
}

export interface RemoteModel {
  id: string
  name?: string
}

export interface ModelTestResult {
  modelId: string
  latencyMs: number
}

export function listServices(): Promise<ServiceList> {
  return get<ServiceList>('/model-services')
}

export function createService(input: ServiceInput): Promise<ServiceDto> {
  return mutate<ServiceDto>('POST', '/model-services', input)
}

export function updateService(id: number, input: ServiceInput): Promise<ServiceDto> {
  return mutate<ServiceDto>('PUT', `/model-services/${id}`, input)
}

export function deleteService(id: number): Promise<void> {
  return mutate<void>('DELETE', `/model-services/${id}`)
}

export function addModel(serviceId: number, input: ModelInput): Promise<ModelDto> {
  return mutate<ModelDto>('POST', `/model-services/${serviceId}/models`, input)
}

export function updateModel(id: number, input: ModelInput): Promise<ModelDto> {
  return mutate<ModelDto>('PUT', `/models/${id}`, input)
}

export function deleteModel(id: number): Promise<void> {
  return mutate<void>('DELETE', `/models/${id}`)
}

// fetchModels is a POST even though it reads: it carries the service URL and
// key in the body and makes the server call the upstream /models endpoint.
// mutate() is what guarantees the CSRF header on that call.
export function fetchModels(input: ProbeInput): Promise<{ models: RemoteModel[] }> {
  return mutate<{ models: RemoteModel[] }>('POST', '/model-services/fetch-models', input)
}

export function testModel(input: TestInput): Promise<ModelTestResult> {
  return mutate<ModelTestResult>('POST', '/models/test', input)
}
