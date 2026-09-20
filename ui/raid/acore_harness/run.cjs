// Browser stubs, so the bundled UI modules load under node for a logic-only check.
// usage: node ui/raid/acore_harness/run.cjs <bundle.cjs> [args...]
const makeElem = () => {
	const elem = {
		style: {},
		dataset: {},
		children: [],
		classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
		setAttribute() {},
		getAttribute: () => null,
		appendChild(child) {
			elem.children.push(child);
			return child;
		},
		prepend() {},
		insertAdjacentHTML() {},
		addEventListener() {},
		removeEventListener() {},
		getElementsByClassName: () => [],
		querySelector: () => null,
		querySelectorAll: () => [],
		remove() {},
		set innerHTML(_v) {},
		get innerHTML() {
			return '';
		},
	};
	return elem;
};

global.window = {
	location: {
		protocol: 'http:',
		host: 'localhost',
		hostname: 'localhost',
		origin: 'http://localhost',
		pathname: '/wotlk/raid/',
		href: 'http://localhost/wotlk/raid/',
		search: '',
		hash: '',
	},
	localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
	// Sim builds a WorkerPool up front. Nothing here runs a sim, so the worker only has to
	// report ready, which SimWorker waits on; onmessage is assigned right after we return.
	Worker: class {
		constructor() {
			setTimeout(() => this.onmessage && this.onmessage({ data: { msg: 'ready' } }), 0);
		}
		postMessage() {}
		terminate() {}
	},
	addEventListener() {},
	matchMedia: () => ({ matches: false, addEventListener() {} }),
	dispatchEvent() {},
};
global.document = Object.assign(makeElem(), {
	createElement: makeElem,
	createDocumentFragment: makeElem,
	getElementById: () => null,
	documentElement: Object.assign(makeElem(), { dir: 'ltr' }),
	readyState: 'complete',
});
global.navigator = { userAgent: 'node', language: 'en-US' };
global.localStorage = global.window.localStorage;
global.Element = class {};
global.HTMLElement = class {};
global.Event = class {};

// Database fetches absolute /wotlk/... paths, which inside the container are the mounted checkout.
const fsNode = require('fs');
global.fetch = async url => {
	const buf = fsNode.readFileSync(String(url));
	return {
		json: async () => JSON.parse(buf.toString('utf-8')),
		// a node Buffer sits in a pooled ArrayBuffer, so hand over just its own bytes
		arrayBuffer: async () => buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength),
	};
};

const bundle = process.argv[2];
if (!bundle) {
	console.error('usage: node ui/raid/acore_harness/run.cjs <bundle.cjs> [args...]');
	process.exit(2);
}
// drop our own argument, so each harness still reads its roster at argv[2]
process.argv.splice(2, 1);
require(require('path').resolve(bundle));
