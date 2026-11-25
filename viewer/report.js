(function () {
  "use strict";
  var form = document.querySelector(".filters");
  var rows = Array.prototype.slice.call(document.querySelectorAll("table.events tbody tr"));
  var shown = document.getElementById("shown");
  if (!form || !rows.length) return;
  form.hidden = false;
  form.addEventListener("submit", function (e) { e.preventDefault(); });
  var q = document.getElementById("q");
  var problems = document.getElementById("problems");
  var total = rows.length;
  var timer = 0;
  function apply() {
    var text = q.value.trim().toLowerCase();
    var levels = {};
    Array.prototype.forEach.call(form.querySelectorAll("input[name=lv]"), function (c) { levels[c.value] = c.checked; });
    var n = 0;
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      var lv = r.getAttribute("data-level");
      var kind = r.getAttribute("data-kind");
      var ok = levels[lv] !== false;
      if (ok && problems.checked) ok = lv === "error" || lv === "fatal" || lv === "warn" || kind === "error" || kind === "crash";
      if (ok && text) ok = r.getAttribute("data-s").indexOf(text) !== -1;
      r.hidden = !ok;
      if (ok) n++;
    }
    shown.textContent = n === total ? total + " shown" : n + " of " + total + " shown";
  }
  form.addEventListener("change", apply);
  q.addEventListener("input", function () { clearTimeout(timer); timer = setTimeout(apply, 120); });
})();
