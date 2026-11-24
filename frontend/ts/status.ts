// the loading notes over the map

type LoadingKey = "heat" | "arrows" | "area" | "testing1" | "testing2";

const loadingIndicatorsKeys: Record<LoadingKey, string> = {
	"heat": "Heatmap loading...",
	"arrows": "Area arrows loading...",
	"area": "Area information loading...",
	"testing1": "Testing1...",
	"testing2": "Testing2...",
};
const loadingIndicators: Record<LoadingKey, boolean> = {
	heat: false,
	arrows: false,
	area: false,
	testing1: false,
	testing2: false,
};
const box = document.getElementById("loadingIndicators") as HTMLDivElement;

export function isLoading(key: LoadingKey): boolean {
	return loadingIndicators[key];
}

export function setLoading(key: LoadingKey, on: boolean) {
	loadingIndicators[key] = on;
	let ret = "";
	for (const v in loadingIndicators) {
		if (loadingIndicators[v as LoadingKey]) {
			ret += loadingIndicatorsKeys[v as LoadingKey] + "<br/>";
		}
	}
	box.innerHTML = ret;
}
