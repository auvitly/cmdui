(() => {
  const dialog = document.querySelector("[data-confirm-dialog]");
  if (!dialog) return;

  const titleNode = dialog.querySelector("[data-confirm-title]");
  const messageNode = dialog.querySelector("[data-confirm-message]");
  const confirmButton = dialog.querySelector("[data-confirm-accept]");
  const cancelButton = dialog.querySelector("[data-confirm-cancel]");
  let pendingForm = null;
  let pendingSubmitter = null;
  const defaultAcceptClass = confirmButton.className;

  document.addEventListener("submit", (event) => {
    if (event.defaultPrevented) return;
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset.confirmBypass === "true") {
      delete form.dataset.confirmBypass;
      return;
    }

    let confirmation = null;
    if (form.hasAttribute("data-confirm-action")) {
      confirmation = {
        title: form.dataset.confirmTitle || "Подтвердите действие",
        message: form.dataset.confirmMessage || "Продолжить выполнение действия?",
        accept: form.dataset.confirmAccept || "Подтвердить",
        variant: form.dataset.confirmVariant || "primary",
      };
    } else if (form.hasAttribute("data-confirm-script")) {
      const commandID = form.elements.namedItem("id")?.value;
      const script = form.querySelector('[name="script"]');
      if (commandID && script && script.value !== script.defaultValue) {
        confirmation = {
          title: "Сохранить изменения скрипта?",
          message: "Текст скрипта изменен. Новая версия будет использоваться при следующих запусках.",
          accept: "Сохранить",
          variant: "primary",
        };
      }
    }

    if (!confirmation) return;
    event.preventDefault();
    pendingForm = form;
    pendingSubmitter = event.submitter || null;
    titleNode.textContent = confirmation.title;
    messageNode.textContent = confirmation.message;
    confirmButton.textContent = confirmation.accept;
    confirmButton.className = `${defaultAcceptClass} ${confirmation.variant === "danger" ? "danger" : "primary"}`;
    dialog.showModal();
  }, true);

  confirmButton.addEventListener("click", () => {
    if (!pendingForm) return;
    const form = pendingForm;
    const submitter = pendingSubmitter;
    pendingForm = null;
    pendingSubmitter = null;
    form.dataset.confirmBypass = "true";
    dialog.close();
    form.requestSubmit(submitter || undefined);
  });

  cancelButton.addEventListener("click", () => dialog.close());

  dialog.addEventListener("close", () => {
    pendingForm = null;
    pendingSubmitter = null;
  });
})();
