export interface HealthResult {
	response: Response;
	version: string | null;
}

export async function fetchHealth(serverUrl: string, init?: RequestInit): Promise<HealthResult> {
	const response = await fetch(`${serverUrl}/health`, init);
	if (!response.ok) return { response, version: null };

	try {
		const body: unknown = await response.json();
		if (typeof body !== 'object' || body === null) return { response, version: null };
		const version = (body as { version?: unknown }).version;
		return { response, version: typeof version === 'string' ? version : null };
	} catch {
		return { response, version: null };
	}
}
