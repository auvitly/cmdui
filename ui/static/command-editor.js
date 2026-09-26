(() => {
  const pastelPalette = ["#DCEBFA", "#DDF2E1", "#FCE8D5", "#F7DDE3", "#E9DFF5", "#F5F0CF", "#D8EFEE", "#E8E4DC"];
  const runStatusLabels = {
    running: "Выполняется",
    success: "Успешно",
    failed: "Ошибка",
    timeout: "Тайм-аут",
    interrupted: "Прервано",
  };
  document.querySelectorAll("[data-run-status]").forEach((statusNode) => {
    const status = [...statusNode.classList].find((name) => runStatusLabels[name]);
    if (status) statusNode.textContent = runStatusLabels[status];
  });
  localizeRunTimestamps();
  const dialog = document.querySelector("#command-dialog");
  if (!dialog) {
    watchActiveRun();
    return;
  }

  const form = dialog.querySelector("form");
  const labelsEditor = dialog.querySelector("#labels-editor");
  const rowTemplate = dialog.querySelector("#label-row-template");
  let paletteIndex = labelsEditor.querySelectorAll(".label-row").length;

  function updateColor(row, color) {
    const normalized = color.toUpperCase();
    row.querySelector("[name=label_color]").value = normalized;
    row.querySelector(".color-dot").style.backgroundColor = normalized;
    row.querySelector("input[type=color]").value = normalized;
    const channels = [0, 2, 4].map((offset) => parseInt(normalized.slice(1 + offset, 3 + offset), 16));
    row.querySelectorAll("[data-rgb]").forEach((slider, index) => {
      slider.value = channels[index];
      slider.closest("label").querySelector("output").value = channels[index];
    });
  }

  function closeColorPopovers(except) {
    dialog.querySelectorAll(".color-popover:not([hidden])").forEach((popover) => {
      if (popover !== except) popover.hidden = true;
    });
  }

  function bindLabelRow(row) {
    const trigger = row.querySelector(".color-trigger");
    const popover = row.querySelector(".color-popover");
    const picker = popover.querySelector("input[type=color]");
    trigger.addEventListener("click", (event) => {
      event.stopPropagation();
      const opening = popover.hidden;
      closeColorPopovers(popover);
      popover.hidden = !opening;
    });
    popover.querySelector(".popover-close").addEventListener("click", () => { popover.hidden = true; });
    picker.addEventListener("input", () => updateColor(row, picker.value));
    popover.querySelectorAll("[data-palette-color]").forEach((button) => button.addEventListener("click", () => {
      updateColor(row, button.dataset.paletteColor);
    }));
    popover.querySelectorAll("[data-rgb]").forEach((slider) => slider.addEventListener("input", () => {
      slider.closest("label").querySelector("output").value = slider.value;
      const channels = [...popover.querySelectorAll("[data-rgb]")].map((item) => Number(item.value));
      updateColor(row, `#${channels.map((channel) => channel.toString(16).padStart(2, "0")).join("")}`);
    }));
    row.querySelector(".remove-label").addEventListener("click", () => row.remove());
    updateColor(row, row.querySelector("[name=label_color]").value || pastelPalette[paletteIndex++ % pastelPalette.length]);
  }

  labelsEditor.querySelectorAll(".label-row").forEach(bindLabelRow);
  dialog.querySelector("#add-label").addEventListener("click", () => {
    const row = rowTemplate.content.firstElementChild.cloneNode(true);
    row.querySelector("[name=label_color]").value = pastelPalette[paletteIndex++ % pastelPalette.length];
    labelsEditor.append(row);
    bindLabelRow(row);
    row.querySelector("[name=label_key]").focus();
  });

  const iconPicker = dialog.querySelector("[data-icon-picker]");
  if (iconPicker) {
    const iconInput = iconPicker.querySelector("[name=icon]");
    const iconTrigger = iconPicker.querySelector(".icon-picker-trigger");
    const iconOptions = [...iconPicker.querySelectorAll(".icon-option")];
    const iconPreview = iconPicker.querySelector("[data-icon-preview]");
    const iconLabel = iconPicker.querySelector("[data-icon-label]");
    const iconMenu = iconPicker.querySelector(".icon-picker-options");
    dialog.append(iconMenu);
    const positionIconMenu = () => {
      if (iconMenu.hidden) return;
      const triggerRect = iconTrigger.getBoundingClientRect();
      const edgePadding = 12;
      const menuWidth = Math.min(390, window.innerWidth - edgePadding * 2);
      const menuHeight = Math.min(iconMenu.scrollHeight, 260);
      const left = Math.min(triggerRect.left, window.innerWidth - menuWidth - edgePadding);
      let top = triggerRect.bottom + 4;
      if (top + menuHeight > window.innerHeight - edgePadding && triggerRect.top - menuHeight - 4 >= edgePadding) {
        top = triggerRect.top - menuHeight - 4;
      }
      iconMenu.style.width = `${menuWidth}px`;
      iconMenu.style.left = `${Math.max(edgePadding, left)}px`;
      iconMenu.style.top = `${top}px`;
    };
    const setIcon = (option) => {
      iconInput.value = option.dataset.iconKey;
      iconLabel.textContent = option.dataset.iconLabel;
      const glyph = option.querySelector(".icon-picker-glyph").firstElementChild;
      iconPreview.replaceChildren(glyph.cloneNode(true));
      iconOptions.forEach((item) => item.setAttribute("aria-selected", String(item === option)));
      iconMenu.hidden = true;
      iconTrigger.setAttribute("aria-expanded", "false");
    };
    const getSelectedOption = () => iconOptions.find((option) => option.dataset.iconKey === iconInput.value) || iconOptions[0];
    setIcon(getSelectedOption());
    iconTrigger.addEventListener("click", () => {
      iconMenu.hidden = !iconMenu.hidden;
      iconTrigger.setAttribute("aria-expanded", String(!iconMenu.hidden));
      if (!iconMenu.hidden) {
        positionIconMenu();
        getSelectedOption().focus();
      }
    });
    dialog.addEventListener("scroll", positionIconMenu, true);
    window.addEventListener("resize", positionIconMenu);
    iconOptions.forEach((option) => option.addEventListener("click", () => setIcon(option)));
    dialog.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !iconMenu.hidden) {
        iconMenu.hidden = true;
        iconTrigger.setAttribute("aria-expanded", "false");
        iconTrigger.focus();
      }
    });
    dialog.addEventListener("click", (event) => {
      if (!iconPicker.contains(event.target)) {
        iconMenu.hidden = true;
        iconTrigger.setAttribute("aria-expanded", "false");
      }
    });
  }

  dialog.addEventListener("click", (event) => {
    if (!event.target.closest(".color-popover") && !event.target.closest(".color-trigger")) closeColorPopovers();
  });
  dialog.querySelectorAll("[data-close-dialog]").forEach((button) => button.addEventListener("click", () => dialog.close()));
  document.querySelector("#new-command").addEventListener("click", () => dialog.showModal());
  if (dialog.hasAttribute("open")) {
    dialog.removeAttribute("open");
    dialog.showModal();
  }
  form.addEventListener("submit", (event) => {
    if (event.defaultPrevented) return;
    const script = form.querySelector("[name=script]").value.trim();
    const program = form.querySelector("[name=program]").value;
    if (!script && !program) {
      event.preventDefault();
      form.querySelector("[name=script]").focus();
      window.alert("Укажите скрипт или выберите исполняемый файл.");
      return;
    }
    for (const row of labelsEditor.querySelectorAll(".label-row")) {
      const key = row.querySelector("[name=label_key]").value.trim();
      const value = row.querySelector("[name=label_value]").value.trim();
      if (!key || !value) {
        event.preventDefault();
        row.querySelector(!key ? "[name=label_key]" : "[name=label_value]").focus();
        window.alert("Заполните ключ и значение label либо удалите строку.");
        return;
      }
    }
  });

  watchActiveRun();

  function watchActiveRun() {
    watchRunAvailability();
    const panels = document.querySelectorAll("[data-run-status-url]");
    panels.forEach((panel) => {
    const statusNode = panel.querySelector("[data-run-status]");
    const interruptForm = panel.querySelector("[data-interrupt-form]");
    const interruptButton = interruptForm?.querySelector("button");
    const interruptNote = panel.querySelector("[data-interrupt-note]");

    if (interruptForm) {
      interruptForm.addEventListener("submit", async (event) => {
        if (event.defaultPrevented) return;
        event.preventDefault();
        if (interruptButton.disabled) return;
        interruptButton.disabled = true;
        interruptButton.textContent = "Прерывание…";
        try {
          const response = await fetch(interruptForm.action, {
            method: "POST",
            headers: { "Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json" },
            body: new URLSearchParams(new FormData(interruptForm)),
          });
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          if (interruptNote) interruptNote.hidden = false;
        } catch (error) {
          interruptButton.disabled = false;
          interruptButton.textContent = "Прервать";
          window.alert(`Не удалось отправить прерывание: ${error.message}`);
        }
      });
    }

    const updateOutput = (selector, value) => {
      let output = panel.querySelector(selector);
      if (!value) {
        if (output) output.hidden = true;
        return;
      }
      if (!output) {
        output = document.createElement("pre");
        output.dataset[selector === "[data-run-stdout]" ? "runStdout" : "runStderr"] = "";
        panel.append(output);
      }
      output.textContent = value;
      output.hidden = false;
    };

    const poll = async () => {
      try {
        const response = await fetch(panel.dataset.runStatusUrl, { headers: { "Accept": "application/json" }, cache: "no-store" });
        if (!response.ok) return;
        const run = await response.json();
        statusNode.textContent = runStatusLabels[run.status] || run.status;
        statusNode.className = `status ${run.status}`;
        const exitCode = panel.querySelector("[data-run-exit-code]");
        if (exitCode) exitCode.textContent = run.exit_code;
        const duration = panel.querySelector("[data-run-duration]");
        if (duration && run.duration) duration.textContent = run.duration;
        updateOutput("[data-run-stdout]", run.stdout);
        updateOutput("[data-run-stderr]", run.stderr);
        if (run.status === "interrupted") {
          panel.remove();
          clearInterval(timer);
          return;
        }
        if (run.status !== "running") {
          panel.querySelector("[data-run-spinner]")?.remove();
          if (interruptForm) interruptForm.remove();
          if (interruptNote) interruptNote.hidden = true;
          clearInterval(timer);
        } else if (run.interrupt_requested && interruptButton) {
          interruptButton.disabled = true;
          interruptButton.textContent = "Прерывание…";
          if (interruptNote) interruptNote.hidden = false;
        }
      } catch {
        // The request can temporarily fail during a server restart.
      }
    };

    const timer = window.setInterval(poll, 700);
    poll();
    });
  }

  function localizeRunTimestamps() {
    const formatter = new Intl.DateTimeFormat(undefined, {
      dateStyle: "short",
      timeStyle: "medium",
    });
    const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    document.querySelectorAll("time[data-local-timestamp]").forEach((element) => {
      const timestamp = new Date(element.dateTime);
      if (Number.isNaN(timestamp.getTime())) return;
      element.textContent = formatter.format(timestamp);
      if (timeZone) element.title = timeZone;
    });
  }

  function watchRunAvailability() {
    const runForms = [...document.querySelectorAll('form[action^="/commands/"][action$="/run"]')];
    if (runForms.length === 0) return;
    const pendingCommandIDs = new Set();
    const commandIDFor = (runForm) => Number(runForm.action.match(/\/commands\/(\d+)\/run$/)?.[1]);
    const updateButtons = (activeCommandIDs) => runForms.forEach((runForm) => {
      const commandID = commandIDFor(runForm);
      const busy = pendingCommandIDs.has(commandID) || activeCommandIDs.has(commandID);
      const button = runForm.querySelector('button[type="submit"]');
      if (!button) return;
      button.disabled = busy;
      button.setAttribute("aria-label", busy ? "Команда уже выполняется" : "Запустить");
      button.title = busy ? "Эта команда уже выполняется" : "Запустить";
    });
    runForms.forEach((runForm) => runForm.addEventListener("submit", () => {
      pendingCommandIDs.add(commandIDFor(runForm));
      updateButtons(new Set());
    }));
    const pollAvailability = async () => {
      try {
        const response = await fetch("/runs/availability", { headers: { "Accept": "application/json" }, cache: "no-store" });
        if (!response.ok) return;
        const availability = await response.json();
        updateButtons(new Set(availability.command_ids || []));
      } catch {
        // Keep the server-rendered state if availability polling fails.
      }
    };
    window.setInterval(pollAvailability, 700);
    pollAvailability();
  }
})();
