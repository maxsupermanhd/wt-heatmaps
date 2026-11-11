// the loading notes over the map

const loadingIndicatorsKeys = {
	"heat": "Heatmap loading...",
	"arrows": "Area arrows loading...",
	"area": "Area information loading...",
	"testing1": "Testing1...",
	"testing2": "Testing2...",
};
const loadingIndicators = {
	heat: false,
	arrows: false,
	area: false,
	testing1: false,
	testing2: false,
};
const box = document.getElementById("loadingIndicators");

export function isLoading(key) {
	return loadingIndicators[key];
}

export function setLoading(key, on) {
	loadingIndicators[key] = on;
	let ret = "";
	for (const v in loadingIndicators) {
		if (loadingIndicators[v]) {
			ret += loadingIndicatorsKeys[v] + "<br/>";
		}
	}
	box.innerHTML = ret;
}
