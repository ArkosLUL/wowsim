const NO_ROUTE = "This sim server can't import from AzerothCore";

// The core raid as the sim server reads it off the AzerothCore server, in acraid's roster JSON.
// A refusal throws the server's own message.
export async function fetchServerRoster(): Promise<string> {
	let response: Response;
	let text: string;
	try {
		response = await fetch('/acoreRoster');
		// the body can still break off after the headers came through
		text = await response.text();
	} catch (e) {
		throw new Error("The sim server didn't answer");
	}

	let reply: any;
	try {
		reply = JSON.parse(text);
	} catch (e) {
		// a server built before the route existed answers with its 404 page
		throw new Error(NO_ROUTE);
	}
	if (!response.ok) {
		throw new Error(typeof reply?.error == 'string' && reply.error ? reply.error : NO_ROUTE);
	}
	return text;
}
