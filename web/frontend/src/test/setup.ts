import "@testing-library/jest-dom/vitest"

Object.defineProperty(window.HTMLElement.prototype, "scrollIntoView", {
	value: () => {},
	writable: true,
})
