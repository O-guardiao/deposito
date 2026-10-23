/* Fisio — pequenos comportamentos de interface (toast e modal). Sem dependências. */
(function () {
  "use strict";
  var toastTimer = null;

  function showToast(msg) {
    var el = document.getElementById("toast");
    if (!el || !msg) return;
    el.textContent = msg;
    el.hidden = false;
    el.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      el.classList.remove("show");
      setTimeout(function () { el.hidden = true; }, 250);
    }, 3500);
  }

  function modal() { return document.getElementById("modal"); }

  function openModal() {
    var m = modal();
    if (!m || m.open) return;
    if (typeof m.showModal === "function") m.showModal(); else m.setAttribute("open", "");
    var first = m.querySelector("select, input, textarea, button");
    if (first) first.focus();
  }

  function closeModal() {
    var m = modal();
    if (!m) return;
    if (m.open && typeof m.close === "function") m.close(); else m.removeAttribute("open");
    var body = document.getElementById("modal-body");
    if (body) body.innerHTML = "";
  }

  document.addEventListener("DOMContentLoaded", function () {
    var flash = document.body.getAttribute("data-flash");
    if (flash) showToast(flash);

    var m = modal();
    if (m) {
      m.addEventListener("click", function (e) { if (e.target === m) closeModal(); });
      m.addEventListener("close", function () {
        var body = document.getElementById("modal-body");
        if (body) body.innerHTML = "";
      });
    }
  });

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-close-modal]");
    if (btn) { e.preventDefault(); closeModal(); }
  });

  document.body.addEventListener("toast", function (e) {
    showToast(e.detail && (e.detail.value || e.detail));
  });
  document.body.addEventListener("closeModal", closeModal);

  document.body.addEventListener("htmx:afterSwap", function (e) {
    if (e.detail && e.detail.target && e.detail.target.id === "modal-body") openModal();
  });

  document.body.addEventListener("htmx:responseError", function (e) {
    var status = e.detail && e.detail.xhr ? e.detail.xhr.status : 0;
    showToast(status === 404 ? "Não encontrado. Atualize a página." : "Erro ao salvar. Tente novamente.");
  });
  document.body.addEventListener("htmx:sendError", function () {
    showToast("Sem conexão com o servidor.");
  });
})();
