export function messageFromError(error: unknown): string {
	if (error instanceof Error && error.message.trim() !== '') {
		return error.message;
	}
	if (typeof error === 'string' && error.trim() !== '') {
		return error;
	}
	return 'Something went wrong.';
}
