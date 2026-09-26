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
  initializeCommandBoard();
  const dialog = document.querySelector("#command-dialog");
  if (!dialog) {
    watchActiveRun();
    return;
  }

  const form = dialog.querySelector("form");
  const iconField = form.querySelector("#command-icon")?.closest(".field");
  const iconLabelText = form.querySelector('label[for="command-icon"]');
  const basicGrid = iconField?.closest(".basic-grid");
  if (iconField && basicGrid) {
    if (iconLabelText) iconLabelText.textContent = "Icon";
    basicGrid.prepend(iconField);
  }
  const permissionGrid = form.querySelector(".permission-grid");
  if (permissionGrid) {
    const operatorView = permissionGrid.querySelector('[name="access_operators"]')?.closest(".permission");
    const operatorExecution = permissionGrid.querySelector('[name="operators_can_run"]')?.closest(".permission");
    const userView = permissionGrid.querySelector('[name="access_all_users"]')?.closest(".permission");
    permissionGrid.querySelectorAll("#specific_users, #specific_operators").forEach((select) => select.closest(".permission")?.remove());
    if (operatorView) operatorView.querySelector("label").lastChild.textContent = " Оператор: просмотр";
    if (operatorExecution) operatorExecution.querySelector("label").lastChild.textContent = " Оператор: исполнение";
    if (userView) userView.querySelector("label").lastChild.textContent = " Пользователь: просмотр";
    [operatorView, operatorExecution, userView].filter(Boolean).forEach((permission) => permissionGrid.append(permission));
  }
  const descriptionInput = form.querySelector("#description");
  if (descriptionInput) {
    const description = document.createElement("textarea");
    description.id = descriptionInput.id;
    description.name = descriptionInput.name;
    description.maxLength = descriptionInput.maxLength;
    description.value = decodeBase64Unicode(descriptionInput.value);
    description.placeholder = descriptionInput.placeholder;
    description.className = descriptionInput.className;
    description.rows = 4;
    descriptionInput.replaceWith(description);
  }
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

  function decodeBase64Unicode(value) {
    if (!value) return "";
    try {
      const bytes = Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
      return new TextDecoder().decode(bytes);
    } catch {
      return value;
    }
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
    let iconOptions = [...iconPicker.querySelectorAll(".icon-option")];
    const initialIconKey = iconInput.value;
    const iconPreview = iconPicker.querySelector("[data-icon-preview]");
    const iconLabel = iconPicker.querySelector("[data-icon-label]");
    const iconMenu = iconPicker.querySelector(".icon-picker-options");
    dialog.append(iconMenu);
    const uploadOption = document.createElement("button");
    uploadOption.className = "icon-option icon-upload-option";
    uploadOption.type = "button";
    uploadOption.role = "option";
    uploadOption.setAttribute("aria-label", "Загрузить SVG-иконку");
    uploadOption.innerHTML = '<span class="icon-picker-glyph icon-upload-glyph">+</span>';
    iconMenu.append(uploadOption);
    const uploadInput = document.createElement("input");
    uploadInput.type = "file";
    uploadInput.accept = "image/svg+xml,.svg";
    uploadInput.hidden = true;
    dialog.append(uploadInput);
    const addCustomIconOption = (icon) => {
      if (iconOptions.some((option) => option.dataset.iconKey === icon.key)) return iconOptions.find((option) => option.dataset.iconKey === icon.key);
      const option = document.createElement("button");
      option.className = "icon-option";
      option.type = "button";
      option.role = "option";
      option.dataset.iconKey = icon.key;
      option.dataset.iconLabel = icon.name;
      option.setAttribute("aria-label", icon.name);
      option.setAttribute("aria-selected", "false");
      option.innerHTML = `<span class="icon-picker-glyph"><img class="app-mark custom-mark" src="/icons/custom/${icon.id}" alt="" aria-hidden="true"></span>`;
      iconMenu.insertBefore(option, uploadOption);
      option.addEventListener("click", () => setIcon(option));
      iconOptions.push(option);
      return option;
    };
    const positionIconMenu = () => {
      if (iconMenu.hidden) return;
      const triggerRect = iconTrigger.getBoundingClientRect();
      const edgePadding = 12;
      const menuWidth = Math.min(180, window.innerWidth - edgePadding * 2);
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
    const loadCustomIcons = async () => {
      try {
        const response = await fetch("/admin/icons", { headers: { "Accept": "application/json" }, cache: "no-store" });
        if (!response.ok) return;
        const icons = await response.json();
        icons.forEach(addCustomIconOption);
        if (initialIconKey) {
          const selectedOption = iconOptions.find((option) => option.dataset.iconKey === initialIconKey);
          if (selectedOption) {
            const wasOpen = !iconMenu.hidden;
            setIcon(selectedOption);
            if (wasOpen) {
              iconMenu.hidden = false;
              iconTrigger.setAttribute("aria-expanded", "true");
            }
          }
        }
      } catch {
        // Built-in icons remain available if custom icons cannot be loaded.
      }
    };
    uploadOption.addEventListener("click", () => uploadInput.click());
    uploadInput.addEventListener("change", async () => {
      const file = uploadInput.files?.[0];
      if (!file) return;
      const defaultName = file.name.replace(/\.svg$/i, "");
      const name = window.prompt("Название SVG-иконки", defaultName)?.trim();
      if (!name) {
        uploadInput.value = "";
        return;
      }
      const formData = new FormData();
      formData.append("csrf", document.querySelector('[name="csrf"]')?.value || "");
      formData.append("name", name);
      formData.append("icon", file);
      try {
        const response = await fetch("/admin/icons", { method: "POST", body: formData, headers: { "Accept": "application/json" } });
        if (!response.ok) throw new Error(await response.text());
        const icon = await response.json();
        setIcon(addCustomIconOption(icon));
      } catch (error) {
        window.alert(error.message || "Не удалось загрузить SVG-иконку.");
      } finally {
        uploadInput.value = "";
      }
    });
    const getSelectedOption = () => iconOptions.find((option) => option.dataset.iconKey === iconInput.value) || iconOptions[0];
    setIcon(getSelectedOption());
    iconTrigger.addEventListener("click", () => {
      loadCustomIcons();
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
    loadCustomIcons();
    dialog.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !iconMenu.hidden) {
        iconMenu.hidden = true;
        iconTrigger.setAttribute("aria-expanded", "false");
        iconTrigger.focus();
      }
    });
    dialog.addEventListener("click", (event) => {
      if (!iconPicker.contains(event.target) && !iconMenu.contains(event.target)) {
        iconMenu.hidden = true;
        iconTrigger.setAttribute("aria-expanded", "false");
      }
    });
  }

  const clearEditQuery = () => {
    const url = new URL(window.location.href);
    if (!url.searchParams.has("edit")) return;
    url.searchParams.delete("edit");
    window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
  };
  dialog.addEventListener("click", (event) => {
    if (!event.target.closest(".color-popover") && !event.target.closest(".color-trigger")) closeColorPopovers();
  });
  dialog.querySelectorAll("[data-close-dialog]").forEach((button) => button.addEventListener("click", () => {
    clearEditQuery();
    dialog.close();
  }));
  dialog.addEventListener("close", clearEditQuery);
  document.querySelector("#new-command").addEventListener("click", () => {
    clearEditQuery();
    dialog.showModal();
  });
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

  function initializeCommandBoard() {
    const tableWrap = document.querySelector(".table-wrap");
    const table = tableWrap?.querySelector("table");
    const rows = [...(table?.querySelectorAll("tbody tr") || [])];
    if (!tableWrap || !table || !rows.length) return;

    tableWrap.classList.add("command-board");
    rows.forEach((row) => {
      const cells = row.querySelectorAll("td");
      if (cells.length > 1) cells[cells.length - 1].classList.add("command-actions-cell");
    });

    const sectionHead = tableWrap.closest(".section")?.querySelector(".section-head");
    if (!sectionHead) return;
    const toolbar = document.createElement("div");
    toolbar.className = "command-toolbar";
    toolbar.innerHTML = '<input class="command-search" type="search" placeholder="Поиск по имени и labels" aria-label="Поиск по имени и labels"><div class="command-layout" role="group" aria-label="Количество колонок"><span class="command-layout-label">Колонок</span><button type="button" data-columns="3" aria-pressed="false">3</button><button type="button" data-columns="4" aria-pressed="true">4</button><button type="button" data-columns="5" aria-pressed="false">5</button></div>';
    sectionHead.after(toolbar);
    const search = toolbar.querySelector(".command-search");
    const layoutButtons = [...toolbar.querySelectorAll("[data-columns]")];
    const commandRows = rows.filter((row) => !row.querySelector(".empty"));
    const setColumns = (columns) => {
      table.style.setProperty("--command-columns", columns);
      layoutButtons.forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.columns === columns)));
    };
    layoutButtons.forEach((button) => button.addEventListener("click", () => setColumns(button.dataset.columns)));
    search.addEventListener("input", () => {
      const query = search.value.trim().toLocaleLowerCase();
      commandRows.forEach((row) => {
        const searchable = row.querySelector("td")?.textContent.toLocaleLowerCase() || "";
        row.hidden = query !== "" && !searchable.includes(query);
      });
    });
    setColumns("4");
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
    const updateTiles = (commandRuns, canInterrupt) => runForms.forEach((runForm) => {
      const commandID = commandIDFor(runForm);
      const runID = commandRuns[String(commandID)];
      const busy = pendingCommandIDs.has(commandID) || Boolean(runID);
      const actionGroup = runForm.closest(".actions") || runForm.parentElement;
      if (!actionGroup) return;
      let state = actionGroup.querySelector(".tile-run-state");
      if (!busy) {
        state?.remove();
        runForm.hidden = false;
        const button = runForm.querySelector('button[type="submit"]');
        if (button) {
          button.disabled = false;
          button.setAttribute("aria-label", "Запустить");
          button.title = "Запустить";
        }
        return;
      }
      runForm.hidden = true;
      if (state) {
        const interruptForm = state.querySelector("form");
        if (interruptForm && runID) interruptForm.action = `/runs/${runID}/interrupt`;
        if (interruptForm || !runID || !canInterrupt) return;
        state.remove();
        state = null;
      }
      state = document.createElement("span");
      state.className = "tile-run-state";
      if (runID && canInterrupt) {
        const interruptForm = document.createElement("form");
        interruptForm.method = "post";
        interruptForm.action = `/runs/${runID}/interrupt`;
        const csrf = document.querySelector('[name="csrf"]');
        if (csrf) {
          const csrfInput = document.createElement("input");
          csrfInput.type = "hidden";
          csrfInput.name = "csrf";
          csrfInput.value = csrf.value;
          interruptForm.append(csrfInput);
        }
        const button = document.createElement("button");
        button.className = "button danger icon-action";
        button.type = "submit";
        button.textContent = "Прервать";
        button.setAttribute("aria-label", "Прервать запуск");
        button.title = "Прервать запуск";
        interruptForm.append(button);
        interruptForm.addEventListener("submit", async (event) => {
          event.preventDefault();
          button.disabled = true;
          button.textContent = "Прерывание…";
          try {
            const response = await fetch(interruptForm.action, {
              method: "POST",
              headers: { "Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json" },
              body: new URLSearchParams(new FormData(interruptForm)),
            });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
          } catch (error) {
            button.disabled = false;
            button.textContent = "Прервать";
            window.alert(`Не удалось прервать запуск: ${error.message}`);
          }
        });
        state.append(interruptForm);
      } else {
        state.classList.add("tile-run-spinner");
        state.innerHTML = '<img src="/static/icons/run-spinner.svg" alt="" aria-hidden="true"><span>Выполняется</span>';
      }
      actionGroup.append(state);
    });
    runForms.forEach((runForm) => runForm.addEventListener("submit", () => {
      pendingCommandIDs.add(commandIDFor(runForm));
      updateTiles({}, false);
    }));
    const pollAvailability = async () => {
      try {
        const response = await fetch("/runs/availability", { headers: { "Accept": "application/json" }, cache: "no-store" });
        if (!response.ok) return;
        const availability = await response.json();
        updateTiles(availability.command_runs || {}, availability.can_interrupt === true);
      } catch {
        // Keep the server-rendered state if availability polling fails.
      }
    };
    window.setInterval(pollAvailability, 700);
    pollAvailability();
  }
})();
