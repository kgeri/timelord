import { TIMETOWER_API_TOKEN, TIMETOWER_API_URL } from '$app/env/private';
import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

const FORWARDED_RESPONSE_HEADERS = [
	'content-type',
	'x-schedule-revision',
	'allow',
	'www-authenticate'
] as const;

/**
 * Same-origin proxy for Timetower.
 *
 * The admin UI can call `/api/v1/...` without exposing the shared bearer key to
 * the browser. The proxy is only used when `PUBLIC_TIMETOWER_MODE=http`; the
 * default build runs entirely on mock data.
 */
const handle: RequestHandler = async ({ request, params, url }) => {
	const upstream = TIMETOWER_API_URL.replace(/\/+$/, '');
	if (!upstream) {
		throw error(503, 'Timetower proxy is not configured. Set TIMETOWER_API_URL.');
	}

	const path = params.path ?? '';
	const target = `${upstream}/api/v1/${path}${url.search}`;

	const headers = new Headers();
	headers.set('Accept', request.headers.get('accept') ?? 'application/json');

	const contentType = request.headers.get('content-type');
	if (contentType) headers.set('Content-Type', contentType);

	const token = TIMETOWER_API_TOKEN;
	if (token) headers.set('Authorization', `Bearer ${token}`);

	const body =
		request.method === 'GET' || request.method === 'HEAD'
			? undefined
			: await request.arrayBuffer();

	let response: Response;
	try {
		response = await fetch(target, {
			method: request.method,
			headers,
			body,
			signal: request.signal
		});
	} catch (cause) {
		const message = cause instanceof Error ? cause.message : 'unknown error';
		throw error(502, `Timetower proxy request failed: ${message}`);
	}

	const responseHeaders = new Headers({ 'Cache-Control': 'no-store' });
	for (const name of FORWARDED_RESPONSE_HEADERS) {
		const value = response.headers.get(name);
		if (value) responseHeaders.set(name, value);
	}

	return new Response(request.method === 'HEAD' ? null : response.body, {
		status: response.status,
		statusText: response.statusText,
		headers: responseHeaders
	});
};

export const GET: RequestHandler = handle;
export const PUT: RequestHandler = handle;
export const POST: RequestHandler = handle;
export const PATCH: RequestHandler = handle;
export const DELETE: RequestHandler = handle;
export const HEAD: RequestHandler = handle;
export const OPTIONS: RequestHandler = handle;
