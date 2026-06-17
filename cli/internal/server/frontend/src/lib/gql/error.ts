type GraphQLErrorLike = {
	message?: string;
	response?: {
		errors?: Array<{
			message?: string;
		}>;
	};
};

export function extractGraphQLErrorMessage(error: unknown, fallback = 'Operation failed'): string {
	if (typeof error === 'string') {
		return error.trim() || fallback;
	}

	if (error && typeof error === 'object') {
		const gqlError = error as GraphQLErrorLike;
		const responseMessages = gqlError.response?.errors
			?.map((entry) => entry.message?.trim())
			.filter((message): message is string => Boolean(message));
		if (responseMessages && responseMessages.length > 0) {
			return responseMessages.join('; ');
		}

		if (typeof gqlError.message === 'string' && gqlError.message.trim()) {
			return gqlError.message.trim();
		}
	}

	return fallback;
}
