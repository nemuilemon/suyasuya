// Application entry point: connect page routing, authentication and shared dialog controls.
import { setupAuth, restoreSession } from "./auth.js";
import { setupRouter } from "./router.js";
import { select } from "./ui.js";
setupRouter();
setupAuth();
select("#close-editor").onclick = select("#back-editor").onclick = () =>
  select("#editor").close();
restoreSession();
