/**
 * An error raised by a Timetower service implementation.
 *
 * `status` is the HTTP status when the failure came from the API, or 0 for a
 * transport/configuration problem. `code` is a stable machine-readable tag for
 * cases the UI may special-case (for example `not_implemented`).
 */
export class ServiceError extends Error {
	readonly status: number;
	readonly code: string | null;

	constructor(message: string, status = 0, code: string | null = null) {
		super(message);
		this.name = 'ServiceError';
		this.status = status;
		this.code = code;
	}
}
