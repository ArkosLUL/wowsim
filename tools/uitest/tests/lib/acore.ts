import type { Page, Request } from '@playwright/test';

// The sim server's own texts (sim/web/acroster/handler.go)
export const NOT_CONFIGURED = "This sim server isn't set up to import from AzerothCore: it was started without --acoreDsn, --acoreDbcDir, --acoreNamesFile";
export const DATABASE_DOWN = "The AzerothCore database didn't answer. The sim server's log says why.";

export interface Reply {
	status?: number;
	contentType?: string;
	body: string;
	// the answer is held back until this resolves
	after?: Promise<void>;
}

export interface StubbedServer {
	reply: Reply;
	requests: Array<Request>;
}

export const errorReply = (status: number, code: string, error: string): Reply => ({ status, body: JSON.stringify({ code, error }) });

// For a test that doesn't care about the AzerothCore importer's fetch on open. A plain dev server
// answers it with a 503, which watchForErrors counts; a dropped request it skips.
export async function dropServerRoster(page: Page) {
	await page.route(/\/acoreRoster(\?.*)?$/, route => route.abort());
}

// Stands in for the sim server's /acoreRoster, which needs a live AzerothCore database. Set `reply`
// to change the next answer. A request with a query string lands here too, so a test can catch one.
export async function stubServer(page: Page, reply: Reply): Promise<StubbedServer> {
	const server: StubbedServer = { reply, requests: [] };
	await page.route(/\/acoreRoster(\?.*)?$/, async route => {
		server.requests.push(route.request());
		const { status = 200, contentType = 'application/json', body, after } = server.reply;
		await after;
		await route.fulfill({ status, contentType, body });
	});
	return server;
}
