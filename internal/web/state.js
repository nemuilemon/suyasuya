// Session and navigation state shared by pages. Timers for PVT stay in pages-pvt.js.
export const appState = {
  user: null,
  settings: null,
  page: location.hash.slice(2) || "records",
  jobTimer: null,
};
