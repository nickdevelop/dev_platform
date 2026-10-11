// Model management API calls. DTOs mirror docs/api/openapi.yaml model schemas
// and the server's modelconfig DTOs; the API key is write-only and never
// returned (services expose hasKey and an opaque secretRef).
import { apiRequest } from './client'

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
  return apiRequest<ServiceList>('GET', '/model-services')
}

export function createService(input: ServiceInput, csrfToken: string): Promise<ServiceDto> {
  return apiRequest<ServiceDto>('POST', '/model-services', { body: input, csrfToken })
}

export function updateService(
  id: number,
  input: ServiceInput,
  csrfToken: string,
): Promise<ServiceDto> {
  return apiRequest<ServiceDto>('PUT', `/model-services/${id}`, { body: input, csrfToken })
}

export function deleteService(id: number, csrfToken: string): Promise<void> {
  return apiRequest<void>('DELETE', `/model-services/${id}`, { csrfToken })
}

export function addModel(
  serviceId: number,
  input: ModelInput,
  csrfToken: string,
): Promise<ModelDto> {
  return apiRequest<ModelDto>('POST', `/model-services/${serviceId}/models`, {
    body: input,
    csrfToken,
  })
}

export function updateModel(id: number, input: ModelInput, csrfToken: string): Promise<ModelDto> {
  return apiRequest<ModelDto>('PUT', `/models/${id}`, { body: input, csrfToken })
}

export function deleteModel(id: number, csrfToken: string): Promise<void> {
  return apiRequest<void>('DELETE', `/models/${id}`, { csrfToken })
}

export function fetchModels(input: ProbeInput): Promise<{ models: RemoteModel[] }> {
  return apiRequest<{ models: RemoteModel[] }>('POST', '/model-services/fetch-models', {
    body: input,
  })
}

export function testModel(input: TestInput): Promise<ModelTestResult> {
  return apiRequest<ModelTestResult>('POST', '/models/test', { body: input })
}
