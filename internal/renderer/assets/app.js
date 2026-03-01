(function () {
  "use strict";

  // ---------------------------------------------------------------------------
  // Constants
  // ---------------------------------------------------------------------------

  var FIELD_TYPE = {
    DOUBLE: 1,
    FLOAT: 2,
    INT64: 3,
    UINT64: 4,
    INT32: 5,
    FIXED64: 6,
    FIXED32: 7,
    BOOL: 8,
    STRING: 9,
    MESSAGE: 11,
    BYTES: 12,
    UINT32: 13,
    ENUM: 14,
    SFIXED32: 15,
    SFIXED64: 16,
    SINT32: 17,
    SINT64: 18,
  };

  var FIELD_LABEL = {
    OPTIONAL: 1,
    REQUIRED: 2,
    REPEATED: 3,
  };

  var FIELD_TYPE_NAMES = {};
  FIELD_TYPE_NAMES[FIELD_TYPE.DOUBLE] = "double";
  FIELD_TYPE_NAMES[FIELD_TYPE.FLOAT] = "float";
  FIELD_TYPE_NAMES[FIELD_TYPE.INT64] = "int64";
  FIELD_TYPE_NAMES[FIELD_TYPE.UINT64] = "uint64";
  FIELD_TYPE_NAMES[FIELD_TYPE.INT32] = "int32";
  FIELD_TYPE_NAMES[FIELD_TYPE.FIXED64] = "fixed64";
  FIELD_TYPE_NAMES[FIELD_TYPE.FIXED32] = "fixed32";
  FIELD_TYPE_NAMES[FIELD_TYPE.BOOL] = "bool";
  FIELD_TYPE_NAMES[FIELD_TYPE.STRING] = "string";
  FIELD_TYPE_NAMES[FIELD_TYPE.MESSAGE] = "message";
  FIELD_TYPE_NAMES[FIELD_TYPE.BYTES] = "bytes";
  FIELD_TYPE_NAMES[FIELD_TYPE.UINT32] = "uint32";
  FIELD_TYPE_NAMES[FIELD_TYPE.ENUM] = "enum";
  FIELD_TYPE_NAMES[FIELD_TYPE.SFIXED32] = "sfixed32";
  FIELD_TYPE_NAMES[FIELD_TYPE.SFIXED64] = "sfixed64";
  FIELD_TYPE_NAMES[FIELD_TYPE.SINT32] = "sint32";
  FIELD_TYPE_NAMES[FIELD_TYPE.SINT64] = "sint64";

  var REQUEST_TIMEOUT_MS = 10000;

  // ---------------------------------------------------------------------------
  // DOM helpers
  // ---------------------------------------------------------------------------

  function el(tag, attrs, children) {
    var node = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (key) {
        if (key === "className") {
          node.className = attrs[key];
        } else if (key === "textContent") {
          node.textContent = attrs[key];
        } else if (key === "innerHTML") {
          node.innerHTML = attrs[key];
        } else if (key.indexOf("on") === 0) {
          node.addEventListener(key.slice(2).toLowerCase(), attrs[key]);
        } else {
          node.setAttribute(key, attrs[key]);
        }
      });
    }
    if (children) {
      if (!Array.isArray(children)) children = [children];
      children.forEach(function (child) {
        if (!child) return;
        if (typeof child === "string") {
          node.appendChild(document.createTextNode(child));
        } else {
          node.appendChild(child);
        }
      });
    }
    return node;
  }

  function text(str) {
    return document.createTextNode(str);
  }

  // ---------------------------------------------------------------------------
  // Field type helpers
  // ---------------------------------------------------------------------------

  function fieldTypeName(field) {
    if (field.isMap) {
      var keyName = FIELD_TYPE_NAMES[field.mapKeyType] || "unknown";
      var valName;
      if (
        field.mapValueType === FIELD_TYPE.MESSAGE ||
        field.mapValueType === FIELD_TYPE.ENUM
      ) {
        valName = shortTypeName(field.mapValueTypeName);
      } else {
        valName = FIELD_TYPE_NAMES[field.mapValueType] || "unknown";
      }
      return "map<" + keyName + ", " + valName + ">";
    }
    if (field.type === FIELD_TYPE.MESSAGE && field.resolvedMessage) {
      return field.resolvedMessage.name;
    }
    if (field.type === FIELD_TYPE.ENUM && field.resolvedEnum) {
      return field.resolvedEnum.name;
    }
    return FIELD_TYPE_NAMES[field.type] || "unknown";
  }

  function shortTypeName(fqn) {
    if (!fqn) return "";
    var parts = fqn.split(".");
    return parts[parts.length - 1];
  }

  function packageFromFullName(fullName) {
    if (!fullName) return "";
    var idx = fullName.lastIndexOf(".");
    return idx >= 0 ? fullName.substring(0, idx) : "";
  }

  function isIntegerType(type) {
    return (
      type === FIELD_TYPE.INT32 ||
      type === FIELD_TYPE.SINT32 ||
      type === FIELD_TYPE.UINT32 ||
      type === FIELD_TYPE.FIXED32 ||
      type === FIELD_TYPE.SFIXED32
    );
  }

  function isLargeIntType(type) {
    return (
      type === FIELD_TYPE.INT64 ||
      type === FIELD_TYPE.UINT64 ||
      type === FIELD_TYPE.SINT64 ||
      type === FIELD_TYPE.FIXED64 ||
      type === FIELD_TYPE.SFIXED64
    );
  }

  function isFloatType(type) {
    return type === FIELD_TYPE.DOUBLE || type === FIELD_TYPE.FLOAT;
  }

  // ---------------------------------------------------------------------------
  // Sidebar rendering
  // ---------------------------------------------------------------------------

  function renderSidebar(schema, sidebar) {
    sidebar.innerHTML = "";

    var nav = el("nav", { className: "sidebar-nav" });

    schema.services.forEach(function (svc) {
      var group = el("div", { className: "sidebar-group" });

      var header = el("div", {
        className: "sidebar-group-header",
        onClick: function () {
          group.classList.toggle("collapsed");
        },
      });

      var arrow = el("span", { className: "sidebar-arrow" });
      arrow.innerHTML =
        '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>';

      header.appendChild(arrow);
      header.appendChild(el("span", { className: "sidebar-service-name", textContent: svc.name }));
      group.appendChild(header);

      var list = el("div", { className: "sidebar-rpc-list" });
      svc.rpcs.forEach(function (rpc) {
        var item = el("div", {
          className: "sidebar-rpc-item",
          "data-rpc-id": rpcId(svc, rpc),
          onClick: function () {
            scrollToRpc(svc, rpc);
          },
        });

        var badge = el("span", {
          className: "method-badge method-badge--" + rpc.httpMethod.toLowerCase(),
          textContent: rpc.httpMethod,
        });

        item.appendChild(badge);
        item.appendChild(el("span", { className: "sidebar-rpc-name", textContent: rpc.name }));
        list.appendChild(item);
      });

      group.appendChild(list);
      nav.appendChild(group);
    });

    sidebar.appendChild(nav);
  }

  function rpcId(svc, rpc) {
    return svc.fullName + "/" + rpc.name;
  }

  function scrollToRpc(svc, rpc) {
    var id = "rpc-" + rpcId(svc, rpc);
    var target = document.getElementById(id);
    if (!target) return;
    target.scrollIntoView({ behavior: "smooth", block: "start" });
    target.classList.add("rpc-card--highlight");
    setTimeout(function () {
      target.classList.remove("rpc-card--highlight");
    }, 1500);
  }

  // ---------------------------------------------------------------------------
  // Sidebar filter
  // ---------------------------------------------------------------------------

  function setupFilter(schema, sidebar) {
    var filterInput = document.getElementById("filter-input");
    if (!filterInput) return;

    filterInput.addEventListener("input", function () {
      var query = filterInput.value.toLowerCase().trim();
      var groups = sidebar.querySelectorAll(".sidebar-group");

      groups.forEach(function (group) {
        var serviceName = group.querySelector(".sidebar-service-name");
        var items = group.querySelectorAll(".sidebar-rpc-item");
        var svcMatch =
          !query || (serviceName && serviceName.textContent.toLowerCase().indexOf(query) >= 0);
        var anyVisible = false;

        items.forEach(function (item) {
          var rpcName = item.querySelector(".sidebar-rpc-name");
          var match =
            svcMatch || (rpcName && rpcName.textContent.toLowerCase().indexOf(query) >= 0);
          item.style.display = match ? "" : "none";
          if (match) anyVisible = true;
        });

        group.style.display = svcMatch || anyVisible ? "" : "none";
        if (query && (svcMatch || anyVisible)) {
          group.classList.remove("collapsed");
        }
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Main content rendering
  // ---------------------------------------------------------------------------

  function renderMain(schema, mainEl) {
    mainEl.innerHTML = "";

    if (!schema.services || schema.services.length === 0) {
      mainEl.appendChild(
        el("div", { className: "empty-state" }, [
          el("p", { textContent: "No services found." }),
        ])
      );
      return;
    }

    schema.services.forEach(function (svc) {
      var section = el("section", { className: "service-section" });

      // Service header
      var header = el("div", { className: "service-header" });
      header.appendChild(el("h2", { className: "service-name", textContent: svc.name }));
      var pkg = packageFromFullName(svc.fullName);
      if (pkg) {
        header.appendChild(
          el("span", { className: "service-package", textContent: pkg })
        );
      }
      if (svc.comment) {
        header.appendChild(el("p", { className: "service-comment", textContent: svc.comment }));
      }
      section.appendChild(header);

      // RPC cards
      svc.rpcs.forEach(function (rpc) {
        section.appendChild(renderRpcCard(svc, rpc));
      });

      mainEl.appendChild(section);
    });
  }

  function renderRpcCard(svc, rpc) {
    var card = el("div", {
      className: "rpc-card",
      id: "rpc-" + rpcId(svc, rpc),
    });

    // Card header row
    var headerRow = el("div", { className: "rpc-card-header" });

    var badge = el("span", {
      className: "method-badge method-badge--" + rpc.httpMethod.toLowerCase(),
      textContent: rpc.httpMethod,
    });
    headerRow.appendChild(badge);
    headerRow.appendChild(el("span", { className: "rpc-name", textContent: rpc.name }));

    // Streaming badges
    if (rpc.clientStreaming) {
      headerRow.appendChild(
        el("span", { className: "streaming-badge", textContent: "client streaming" })
      );
    }
    if (rpc.serverStreaming) {
      headerRow.appendChild(
        el("span", { className: "streaming-badge", textContent: "server streaming" })
      );
    }

    card.appendChild(headerRow);

    // Path
    card.appendChild(
      el("div", { className: "rpc-path", textContent: rpc.connectPath })
    );

    // Comment
    if (rpc.comment) {
      card.appendChild(el("p", { className: "rpc-comment", textContent: rpc.comment }));
    }

    // Request section
    if (rpc.request && rpc.request.resolved) {
      card.appendChild(renderMessageSection("REQUEST", rpc.request.resolved));
    }

    // Response section
    if (rpc.response && rpc.response.resolved) {
      card.appendChild(renderMessageSection("RESPONSE", rpc.response.resolved));
    }

    // Try It button and panel
    var tryItBtn = el("button", {
      className: "try-it-btn",
      textContent: "Try it",
      onClick: function () {
        var panel = card.querySelector(".try-it-panel");
        if (panel) {
          panel.classList.toggle("try-it-panel--open");
          tryItBtn.classList.toggle("try-it-btn--active");
        }
      },
    });
    card.appendChild(tryItBtn);
    card.appendChild(renderTryItPanel(svc, rpc));

    return card;
  }

  // ---------------------------------------------------------------------------
  // Message / field tree rendering
  // ---------------------------------------------------------------------------

  function renderMessageSection(label, message) {
    var section = el("div", { className: "message-section" });
    section.appendChild(
      el("div", { className: "message-section-label", textContent: label })
    );

    var nameRow = el("div", { className: "message-name-row" });
    nameRow.appendChild(el("span", { className: "message-name", textContent: message.name }));
    if (message.comment) {
      nameRow.appendChild(
        el("span", { className: "message-comment", textContent: message.comment })
      );
    }
    section.appendChild(nameRow);

    if (message.fields && message.fields.length > 0) {
      section.appendChild(renderFieldTree(message.fields, 0));
    }

    return section;
  }

  function renderFieldTree(fields, depth) {
    var container = el("div", {
      className: depth > 0 ? "field-children" : "field-tree",
    });

    // Group fields by oneof
    var rendered = {};
    var oneofGroups = {};

    fields.forEach(function (field) {
      if (field.oneofName && field.oneofName !== "") {
        if (!oneofGroups[field.oneofName]) {
          oneofGroups[field.oneofName] = [];
        }
        oneofGroups[field.oneofName].push(field);
      }
    });

    fields.forEach(function (field) {
      // Oneof grouping
      if (field.oneofName && field.oneofName !== "") {
        if (rendered[field.oneofName]) return;
        rendered[field.oneofName] = true;
        container.appendChild(
          renderOneofGroup(field.oneofName, oneofGroups[field.oneofName], depth)
        );
        return;
      }

      var node = el("div", { className: "field-node" });
      renderFieldIntoNode(node, field, depth);
      container.appendChild(node);
    });

    return container;
  }

  function renderOneofGroup(name, fields, depth) {
    var group = el("div", { className: "oneof-group" });

    var header = el("div", { className: "oneof-header" });
    header.appendChild(
      el("span", { className: "oneof-label", textContent: "oneof " })
    );
    header.appendChild(
      el("span", { className: "oneof-name", textContent: name })
    );
    group.appendChild(header);

    var body = el("div", { className: "oneof-body" });
    fields.forEach(function (field) {
      var node = el("div", { className: "field-node" });
      renderFieldIntoNode(node, field, depth + 1);
      body.appendChild(node);
    });
    group.appendChild(body);

    return group;
  }

  function renderFieldIntoNode(node, field, depth) {
    var row = el("div", { className: "field-row" });

    // Type name
    var typeName = fieldTypeName(field);
    var typeSpan = el("span", { className: "field-type", textContent: typeName });
    row.appendChild(typeSpan);

    // Field name
    var nameSpan = el("span", { className: "field-name", textContent: " " + field.name });
    row.appendChild(nameSpan);

    // Repeated marker
    if (field.label === FIELD_LABEL.REPEATED && !field.isMap) {
      row.appendChild(el("span", { className: "field-repeated", textContent: " []" }));
    }

    // Optional marker
    if (field.isOptional) {
      row.appendChild(el("span", { className: "field-optional", textContent: " ?" }));
    }

    // Comment
    if (field.comment) {
      row.appendChild(
        el("span", { className: "field-comment", textContent: " // " + field.comment })
      );
    }

    node.appendChild(row);

    // Recursive field
    if (field.isRecursive) {
      row.appendChild(el("span", {
        className: "field-recursive",
        textContent: " \u21BB [recursive]",
      }));

      // Expand button for one-level expansion
      if (field.resolvedMessage && field.resolvedMessage.fields) {
        var expandBtn = el("button", {
          className: "field-expand-btn",
          textContent: "expand",
          onClick: function (e) {
            e.stopPropagation();
            var existingChildren = node.querySelector(".field-children");
            if (existingChildren) {
              existingChildren.remove();
              expandBtn.textContent = "expand";
            } else {
              node.appendChild(renderFieldTree(field.resolvedMessage.fields, depth + 1));
              expandBtn.textContent = "collapse";
            }
          },
        });
        row.appendChild(expandBtn);
      }
      return;
    }

    // Nested message fields (non-recursive)
    if (
      field.type === FIELD_TYPE.MESSAGE &&
      field.resolvedMessage &&
      field.resolvedMessage.fields &&
      field.resolvedMessage.fields.length > 0 &&
      !field.isMap
    ) {
      var toggle = el("button", { className: "field-toggle", "aria-expanded": "true" });
      toggle.innerHTML = "\u25BC";
      row.insertBefore(toggle, row.firstChild);

      var children = renderFieldTree(field.resolvedMessage.fields, depth + 1);
      node.appendChild(children);

      toggle.addEventListener("click", function (e) {
        e.stopPropagation();
        var expanded = toggle.getAttribute("aria-expanded") === "true";
        if (expanded) {
          children.style.display = "none";
          toggle.innerHTML = "\u25B6";
          toggle.setAttribute("aria-expanded", "false");
        } else {
          children.style.display = "";
          toggle.innerHTML = "\u25BC";
          toggle.setAttribute("aria-expanded", "true");
        }
      });
      return;
    }

    // Enum values
    if (field.type === FIELD_TYPE.ENUM && field.resolvedEnum && field.resolvedEnum.values) {
      var enumVals = el("div", { className: "enum-values" });
      field.resolvedEnum.values.forEach(function (v) {
        var valRow = el("span", {
          className: "enum-value",
          textContent: v.name + " = " + v.number,
        });
        if (v.comment) {
          valRow.appendChild(
            el("span", { className: "enum-value-comment", textContent: " // " + v.comment })
          );
        }
        enumVals.appendChild(valRow);
      });
      node.appendChild(enumVals);
    }
  }

  // ---------------------------------------------------------------------------
  // Try It panel
  // ---------------------------------------------------------------------------

  function renderTryItPanel(svc, rpc) {
    var panel = el("div", { className: "try-it-panel" });

    // Headers section
    var headersSection = el("div", { className: "try-it-section" });
    headersSection.appendChild(
      el("div", { className: "try-it-section-label", textContent: "Headers" })
    );
    var headersContainer = el("div", { className: "headers-container" });
    headersSection.appendChild(headersContainer);

    var addHeaderBtn = el("button", {
      className: "add-btn",
      textContent: "+ Add Header",
      onClick: function () {
        headersContainer.appendChild(createHeaderRow());
      },
    });
    headersSection.appendChild(addHeaderBtn);
    panel.appendChild(headersSection);

    // Request body section
    if (rpc.request && rpc.request.resolved && rpc.request.resolved.fields) {
      var bodySection = el("div", { className: "try-it-section" });
      bodySection.appendChild(
        el("div", { className: "try-it-section-label", textContent: "Request Body" })
      );
      var formContainer = el("div", { className: "form-container" });
      rpc.request.resolved.fields.forEach(function (field) {
        formContainer.appendChild(buildFormField(field, 0));
      });
      bodySection.appendChild(formContainer);
      panel.appendChild(bodySection);
    }

    // Action row
    var actionRow = el("div", { className: "try-it-actions" });

    var sendBtn = el("button", {
      className: "send-btn",
      textContent: "Send",
      onClick: function () {
        handleSend(rpc, panel);
      },
    });
    actionRow.appendChild(sendBtn);

    var curlBtn = el("button", {
      className: "curl-btn",
      textContent: "Copy as curl",
      onClick: function () {
        handleCopyCurl(rpc, panel, curlBtn);
      },
    });
    actionRow.appendChild(curlBtn);

    panel.appendChild(actionRow);

    // Response area
    var responseArea = el("div", { className: "response-area" });
    panel.appendChild(responseArea);

    return panel;
  }

  function createHeaderRow(key, value) {
    var row = el("div", { className: "header-row" });

    var keyInput = el("input", {
      type: "text",
      className: "header-key-input",
      placeholder: "Header name",
      value: key || "",
    });

    var valInput = el("input", {
      type: "text",
      className: "header-value-input",
      placeholder: "Value",
      value: value || "",
    });

    var removeBtn = el("button", {
      className: "remove-btn",
      textContent: "\u00D7",
      title: "Remove",
      onClick: function () {
        row.remove();
      },
    });

    row.appendChild(keyInput);
    row.appendChild(valInput);
    row.appendChild(removeBtn);

    return row;
  }

  // ---------------------------------------------------------------------------
  // Form field builders
  // ---------------------------------------------------------------------------

  function buildFormField(field, depth) {
    // Handle oneof at the form level is done by the caller grouping
    if (field.isMap) {
      return buildMapField(field, depth);
    }
    if (field.label === FIELD_LABEL.REPEATED && !field.isMap) {
      return buildRepeatedField(field, depth);
    }
    return buildSingleField(field, depth);
  }

  function buildSingleField(field, depth) {
    var wrapper = el("div", {
      className: "form-field",
      "data-field-name": field.name,
      "data-field-type": field.type,
    });

    var labelRow = el("div", { className: "form-field-label" });
    labelRow.appendChild(el("span", { className: "form-field-name", textContent: field.name }));
    var typeName = fieldTypeName(field);
    labelRow.appendChild(el("span", { className: "form-field-type", textContent: typeName }));
    if (field.isOptional) {
      labelRow.appendChild(el("span", { className: "form-field-optional", textContent: "optional" }));
    }
    if (field.comment) {
      labelRow.appendChild(
        el("span", { className: "form-field-comment", textContent: field.comment })
      );
    }
    wrapper.appendChild(labelRow);

    // Enum
    if (field.type === FIELD_TYPE.ENUM && field.resolvedEnum) {
      var select = el("select", {
        className: "form-input form-select",
        "data-field-name": field.name,
      });
      select.appendChild(el("option", { value: "", textContent: "-- select --" }));
      field.resolvedEnum.values.forEach(function (v) {
        select.appendChild(el("option", { value: v.name, textContent: v.name + " (" + v.number + ")" }));
      });
      wrapper.appendChild(select);
      return wrapper;
    }

    // Bool
    if (field.type === FIELD_TYPE.BOOL) {
      var checkbox = el("input", {
        type: "checkbox",
        className: "form-checkbox",
        "data-field-name": field.name,
      });
      wrapper.appendChild(checkbox);
      return wrapper;
    }

    // Message (nested)
    if (field.type === FIELD_TYPE.MESSAGE && field.resolvedMessage) {
      if (field.isRecursive) {
        // Recursive: show an expand button
        var expandBtn = el("button", {
          className: "add-btn",
          textContent: "+ Expand " + field.resolvedMessage.name,
          onClick: function () {
            if (wrapper.querySelector(".form-nested")) {
              wrapper.querySelector(".form-nested").remove();
              expandBtn.textContent = "+ Expand " + field.resolvedMessage.name;
            } else {
              var nested = buildNestedMessageForm(field.resolvedMessage, depth + 1);
              wrapper.appendChild(nested);
              expandBtn.textContent = "- Collapse " + field.resolvedMessage.name;
            }
          },
        });
        wrapper.appendChild(expandBtn);
        return wrapper;
      }

      wrapper.appendChild(buildNestedMessageForm(field.resolvedMessage, depth + 1));
      return wrapper;
    }

    // Scalar inputs
    var input = buildScalarInput(field);
    wrapper.appendChild(input);
    return wrapper;
  }

  function buildScalarInput(field) {
    var type = field.type;

    if (isIntegerType(type)) {
      return el("input", {
        type: "number",
        step: "1",
        className: "form-input",
        "data-field-name": field.name,
        placeholder: FIELD_TYPE_NAMES[type] || "",
      });
    }

    if (isLargeIntType(type)) {
      return el("input", {
        type: "text",
        className: "form-input",
        "data-field-name": field.name,
        placeholder: (FIELD_TYPE_NAMES[type] || "") + " (as string for large numbers)",
      });
    }

    if (isFloatType(type)) {
      return el("input", {
        type: "number",
        step: "any",
        className: "form-input",
        "data-field-name": field.name,
        placeholder: FIELD_TYPE_NAMES[type] || "",
      });
    }

    if (type === FIELD_TYPE.BYTES) {
      return el("input", {
        type: "text",
        className: "form-input",
        "data-field-name": field.name,
        placeholder: "base64-encoded bytes",
      });
    }

    // Default: string
    return el("input", {
      type: "text",
      className: "form-input",
      "data-field-name": field.name,
      placeholder: FIELD_TYPE_NAMES[type] || "string",
    });
  }

  function buildNestedMessageForm(message, depth) {
    var fieldset = el("fieldset", { className: "form-nested" });
    var legend = el("legend", { textContent: message.name });
    fieldset.appendChild(legend);

    // Handle oneof groups in form
    var oneofGroups = {};
    var rendered = {};

    if (message.fields) {
      message.fields.forEach(function (f) {
        if (f.oneofName && f.oneofName !== "") {
          if (!oneofGroups[f.oneofName]) oneofGroups[f.oneofName] = [];
          oneofGroups[f.oneofName].push(f);
        }
      });

      message.fields.forEach(function (f) {
        if (f.oneofName && f.oneofName !== "") {
          if (rendered[f.oneofName]) return;
          rendered[f.oneofName] = true;
          fieldset.appendChild(buildOneofFormGroup(f.oneofName, oneofGroups[f.oneofName], depth));
          return;
        }
        fieldset.appendChild(buildFormField(f, depth));
      });
    }

    return fieldset;
  }

  function buildOneofFormGroup(name, fields, depth) {
    var group = el("div", {
      className: "form-oneof-group",
      "data-oneof-name": name,
    });

    group.appendChild(
      el("div", { className: "form-oneof-label", textContent: "oneof " + name })
    );

    var radioName = "oneof-" + name + "-" + Math.random().toString(36).slice(2, 8);
    var fieldsContainer = el("div", { className: "form-oneof-fields" });

    fields.forEach(function (field, idx) {
      var option = el("div", { className: "form-oneof-option" });

      var radio = el("input", {
        type: "radio",
        name: radioName,
        value: field.name,
        className: "form-oneof-radio",
        "data-oneof-name": name,
      });

      var radioLabel = el("label", { className: "form-oneof-radio-label" });
      radioLabel.appendChild(radio);
      radioLabel.appendChild(text(" " + field.name));
      option.appendChild(radioLabel);

      var fieldContent = el("div", {
        className: "form-oneof-field-content",
        style: "display:none",
      });
      fieldContent.appendChild(buildFormField(field, depth));
      option.appendChild(fieldContent);

      radio.addEventListener("change", function () {
        // Hide all oneof field contents in this group
        var allOptions = group.querySelectorAll(".form-oneof-option");
        allOptions.forEach(function (opt) {
          var content = opt.querySelector(".form-oneof-field-content");
          if (content) content.style.display = "none";
        });
        // Show selected
        fieldContent.style.display = "";
      });

      fieldsContainer.appendChild(option);
    });

    group.appendChild(fieldsContainer);
    return group;
  }

  function buildRepeatedField(field, depth) {
    var wrapper = el("div", {
      className: "form-field form-field--repeated",
      "data-field-name": field.name,
      "data-field-type": field.type,
      "data-is-repeated": "true",
    });

    var labelRow = el("div", { className: "form-field-label" });
    labelRow.appendChild(el("span", { className: "form-field-name", textContent: field.name }));
    labelRow.appendChild(
      el("span", { className: "form-field-type", textContent: fieldTypeName(field) + "[]" })
    );
    if (field.comment) {
      labelRow.appendChild(
        el("span", { className: "form-field-comment", textContent: field.comment })
      );
    }
    wrapper.appendChild(labelRow);

    var itemsContainer = el("div", { className: "repeated-items" });
    wrapper.appendChild(itemsContainer);

    var addBtn = el("button", {
      className: "add-btn",
      textContent: "+ Add item",
      onClick: function () {
        var itemWrapper = el("div", { className: "repeated-item" });

        // Clone the field without repeated label for individual items
        var singleField = Object.assign({}, field);
        singleField.label = FIELD_LABEL.OPTIONAL;
        var fieldEl = buildSingleField(singleField, depth);
        // Remove the label row from individual items (keep it compact)
        var itemLabel = fieldEl.querySelector(".form-field-label");
        if (itemLabel) itemLabel.remove();

        var removeBtn = el("button", {
          className: "remove-btn",
          textContent: "\u00D7",
          title: "Remove",
          onClick: function () {
            itemWrapper.remove();
          },
        });

        itemWrapper.appendChild(fieldEl);
        itemWrapper.appendChild(removeBtn);
        itemsContainer.appendChild(itemWrapper);
      },
    });
    wrapper.appendChild(addBtn);

    return wrapper;
  }

  function buildMapField(field, depth) {
    var wrapper = el("div", {
      className: "form-field form-field--map",
      "data-field-name": field.name,
      "data-is-map": "true",
    });

    var labelRow = el("div", { className: "form-field-label" });
    labelRow.appendChild(el("span", { className: "form-field-name", textContent: field.name }));
    labelRow.appendChild(
      el("span", { className: "form-field-type", textContent: fieldTypeName(field) })
    );
    if (field.comment) {
      labelRow.appendChild(
        el("span", { className: "form-field-comment", textContent: field.comment })
      );
    }
    wrapper.appendChild(labelRow);

    var entriesContainer = el("div", { className: "map-entries" });
    wrapper.appendChild(entriesContainer);

    var addBtn = el("button", {
      className: "add-btn",
      textContent: "+ Add entry",
      onClick: function () {
        entriesContainer.appendChild(createMapEntryRow(field, depth));
      },
    });
    wrapper.appendChild(addBtn);

    return wrapper;
  }

  function createMapEntryRow(field, depth) {
    var row = el("div", { className: "map-entry-row" });

    var keyInput = el("input", {
      type: isIntegerType(field.mapKeyType) ? "number" : "text",
      className: "form-input map-key-input",
      placeholder: "key (" + (FIELD_TYPE_NAMES[field.mapKeyType] || "string") + ")",
    });
    if (isIntegerType(field.mapKeyType)) {
      keyInput.setAttribute("step", "1");
    }

    row.appendChild(keyInput);

    // Value input depends on map value type
    if (field.mapValueType === FIELD_TYPE.MESSAGE) {
      // For message value types, we would need the resolved message
      // For now use a JSON text input
      row.appendChild(
        el("input", {
          type: "text",
          className: "form-input map-value-input",
          placeholder: "value (JSON object)",
        })
      );
    } else if (field.mapValueType === FIELD_TYPE.ENUM) {
      row.appendChild(
        el("input", {
          type: "text",
          className: "form-input map-value-input",
          placeholder: "value (enum name)",
        })
      );
    } else {
      row.appendChild(
        el("input", {
          type: isIntegerType(field.mapValueType) ? "number" : "text",
          className: "form-input map-value-input",
          placeholder: "value (" + (FIELD_TYPE_NAMES[field.mapValueType] || "string") + ")",
        })
      );
    }

    var removeBtn = el("button", {
      className: "remove-btn",
      textContent: "\u00D7",
      title: "Remove",
      onClick: function () {
        row.remove();
      },
    });
    row.appendChild(removeBtn);

    return row;
  }

  // ---------------------------------------------------------------------------
  // Form data collection
  // ---------------------------------------------------------------------------

  function collectFormData(container, fields) {
    var data = {};
    if (!fields) return data;

    var oneofProcessed = {};

    fields.forEach(function (field) {
      // Handle oneof
      if (field.oneofName && field.oneofName !== "") {
        if (oneofProcessed[field.oneofName]) return;
        oneofProcessed[field.oneofName] = true;

        var oneofGroup = container.querySelector(
          '[data-oneof-name="' + field.oneofName + '"]'
        );
        if (!oneofGroup) return;

        var selectedRadio = oneofGroup.querySelector(".form-oneof-radio:checked");
        if (!selectedRadio) return;

        var selectedFieldName = selectedRadio.value;
        var selectedField = fields.find(function (f) {
          return f.name === selectedFieldName;
        });
        if (!selectedField) return;

        var selectedContent = selectedRadio
          .closest(".form-oneof-option")
          .querySelector(".form-oneof-field-content");
        if (!selectedContent) return;

        var value = collectSingleFieldValue(selectedContent, selectedField);
        if (value !== undefined && value !== null && value !== "" && value !== false) {
          data[selectedField.name] = value;
        }
        return;
      }

      var value = collectFieldValue(container, field);
      if (value !== undefined && value !== null && value !== "" && value !== false) {
        data[field.name] = value;
      }
    });

    return data;
  }

  function collectFieldValue(container, field) {
    if (field.isMap) {
      return collectMapValue(container, field);
    }
    if (field.label === FIELD_LABEL.REPEATED && !field.isMap) {
      return collectRepeatedValue(container, field);
    }
    return collectSingleFieldValue(container, field);
  }

  function collectSingleFieldValue(container, field) {
    // Bool
    if (field.type === FIELD_TYPE.BOOL) {
      var checkbox = container.querySelector(
        '[data-field-name="' + field.name + '"].form-checkbox'
      );
      if (!checkbox) {
        // Try inside a form-field wrapper
        var wrapper = findFormFieldWrapper(container, field.name);
        if (wrapper) checkbox = wrapper.querySelector(".form-checkbox");
      }
      return checkbox ? checkbox.checked : false;
    }

    // Enum
    if (field.type === FIELD_TYPE.ENUM) {
      var select = container.querySelector(
        'select[data-field-name="' + field.name + '"]'
      );
      if (!select) {
        var wrapper = findFormFieldWrapper(container, field.name);
        if (wrapper) select = wrapper.querySelector("select");
      }
      return select && select.value ? select.value : undefined;
    }

    // Message
    if (field.type === FIELD_TYPE.MESSAGE && field.resolvedMessage) {
      var wrapper = findFormFieldWrapper(container, field.name);
      if (!wrapper) return undefined;
      var nested = wrapper.querySelector(".form-nested");
      if (!nested) return undefined;
      var value = collectFormData(nested, field.resolvedMessage.fields);
      return Object.keys(value).length > 0 ? value : undefined;
    }

    // Scalars
    var input = container.querySelector(
      'input[data-field-name="' + field.name + '"].form-input'
    );
    if (!input) {
      var wrapper = findFormFieldWrapper(container, field.name);
      if (wrapper) input = wrapper.querySelector("input.form-input");
    }
    if (!input || !input.value) return undefined;

    return parseScalarValue(field.type, input.value);
  }

  function collectRepeatedValue(container, field) {
    var wrapper = findFormFieldWrapper(container, field.name);
    if (!wrapper) return undefined;

    var items = wrapper.querySelectorAll(".repeated-item");
    if (items.length === 0) return undefined;

    var values = [];
    items.forEach(function (item) {
      var singleField = Object.assign({}, field);
      singleField.label = FIELD_LABEL.OPTIONAL;
      var val = collectSingleFieldValue(item, singleField);
      if (val !== undefined && val !== null) {
        values.push(val);
      }
    });

    return values.length > 0 ? values : undefined;
  }

  function collectMapValue(container, field) {
    var wrapper = findFormFieldWrapper(container, field.name);
    if (!wrapper) return undefined;

    var entries = wrapper.querySelectorAll(".map-entry-row");
    if (entries.length === 0) return undefined;

    var map = {};
    entries.forEach(function (entry) {
      var keyInput = entry.querySelector(".map-key-input");
      var valInput = entry.querySelector(".map-value-input");
      if (keyInput && valInput && keyInput.value) {
        var key = keyInput.value;
        var val = valInput.value;
        // Try to parse value based on map value type
        if (field.mapValueType === FIELD_TYPE.MESSAGE) {
          try {
            val = JSON.parse(val);
          } catch (e) {
            // Keep as string if not valid JSON
          }
        } else {
          val = parseScalarValue(field.mapValueType, val);
        }
        map[key] = val;
      }
    });

    return Object.keys(map).length > 0 ? map : undefined;
  }

  function findFormFieldWrapper(container, fieldName) {
    var wrappers = container.querySelectorAll('.form-field[data-field-name="' + fieldName + '"]');
    // Return the first direct child or closest match
    for (var i = 0; i < wrappers.length; i++) {
      // Avoid nested fields with the same name from a deeper message
      if (wrappers[i].closest(".form-nested") === container.closest(".form-nested") ||
          !wrappers[i].closest(".form-nested")) {
        return wrappers[i];
      }
    }
    return wrappers[0] || null;
  }

  function parseScalarValue(type, strValue) {
    if (!strValue && strValue !== "0" && strValue !== "false") return undefined;

    if (isIntegerType(type)) {
      var n = parseInt(strValue, 10);
      return isNaN(n) ? undefined : n;
    }

    if (isLargeIntType(type)) {
      // Return as string for JSON compatibility with large numbers
      return strValue;
    }

    if (isFloatType(type)) {
      var f = parseFloat(strValue);
      return isNaN(f) ? undefined : f;
    }

    if (type === FIELD_TYPE.BOOL) {
      return strValue === "true" || strValue === "1";
    }

    // string, bytes, etc.
    return strValue;
  }

  // ---------------------------------------------------------------------------
  // Collect headers from the panel
  // ---------------------------------------------------------------------------

  function collectHeaders(panel) {
    var headers = {};
    var rows = panel.querySelectorAll(".header-row");
    rows.forEach(function (row) {
      var key = row.querySelector(".header-key-input");
      var val = row.querySelector(".header-value-input");
      if (key && val && key.value.trim()) {
        headers[key.value.trim()] = val.value;
      }
    });
    return headers;
  }

  // ---------------------------------------------------------------------------
  // Request sending
  // ---------------------------------------------------------------------------

  function getBaseURL() {
    if (window.__CONNECTVIEW_SERVE_MODE__) {
      return "/proxy";
    }
    var input = document.getElementById("base-url-input");
    var url = input ? input.value.trim() : "http://localhost:8080";
    // Remove trailing slash
    return url.replace(/\/+$/, "");
  }

  async function sendRequest(rpc, body, headers, baseURL) {
    var url = baseURL + rpc.connectPath;
    var method = rpc.httpMethod;
    var start = performance.now();

    if (method === "GET") {
      var encoded = encodeURIComponent(JSON.stringify(body));
      var getUrl = url + "?encoding=json&message=" + encoded;
      var fetchHeaders = Object.assign({ "Connect-Protocol-Version": "1" }, headers);

      var resp = await fetch(getUrl, {
        method: "GET",
        headers: fetchHeaders,
        signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
      });

      var elapsed = Math.round(performance.now() - start);
      var respBody;
      var clone = resp.clone();
      try {
        respBody = await resp.json();
      } catch (e) {
        respBody = await clone.text().catch(function () {
          return "";
        });
      }

      return {
        status: resp.status,
        statusText: resp.statusText,
        body: respBody,
        elapsed: elapsed,
      };
    }

    // POST
    var fetchHeaders = Object.assign(
      {
        "Content-Type": "application/json",
        "Connect-Protocol-Version": "1",
      },
      headers
    );

    var resp = await fetch(url, {
      method: "POST",
      headers: fetchHeaders,
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });

    var elapsed = Math.round(performance.now() - start);
    var respBody;
    var clone = resp.clone();
    try {
      respBody = await resp.json();
    } catch (e) {
      respBody = await clone.text().catch(function () {
        return "";
      });
    }

    return {
      status: resp.status,
      statusText: resp.statusText,
      body: respBody,
      elapsed: elapsed,
    };
  }

  // ---------------------------------------------------------------------------
  // Handle Send button
  // ---------------------------------------------------------------------------

  async function handleSend(rpc, panel) {
    var responseArea = panel.querySelector(".response-area");
    responseArea.innerHTML = "";
    responseArea.className = "response-area";

    var formContainer = panel.querySelector(".form-container");
    var body = {};
    if (formContainer && rpc.request && rpc.request.resolved) {
      body = collectFormData(formContainer, rpc.request.resolved.fields);
    }

    var headers = collectHeaders(panel);
    var baseURL = getBaseURL();

    // Show loading
    responseArea.appendChild(
      el("div", { className: "response-loading", textContent: "Sending request..." })
    );

    try {
      var result = await sendRequest(rpc, body, headers, baseURL);

      responseArea.innerHTML = "";

      // Status line
      var isSuccess = result.status >= 200 && result.status < 300;
      var statusClass = isSuccess ? "response-status--success" : "response-status--error";
      var statusLine = el("div", { className: "response-status " + statusClass });
      statusLine.appendChild(
        el("span", {
          className: "response-status-code",
          textContent: result.status + " " + result.statusText,
        })
      );
      statusLine.appendChild(
        el("span", { className: "response-elapsed", textContent: result.elapsed + "ms" })
      );
      responseArea.appendChild(statusLine);

      // Response body
      var bodyStr =
        typeof result.body === "string"
          ? result.body
          : JSON.stringify(result.body, null, 2);

      var pre = el("pre", { className: "response-body" });
      pre.appendChild(el("code", null, [syntaxHighlightJSON(bodyStr)]));
      responseArea.appendChild(pre);
    } catch (err) {
      responseArea.innerHTML = "";

      if (err.name === "TimeoutError") {
        responseArea.className = "response-area response-area--error";
        responseArea.appendChild(
          el("div", {
            className: "response-error",
            textContent: "Request timed out after " + REQUEST_TIMEOUT_MS / 1000 + "s",
          })
        );
      } else {
        responseArea.className = "response-area response-area--error";
        var errorMsg = el("div", { className: "response-error" });
        errorMsg.appendChild(
          el("div", { textContent: "Network Error: " + (err.message || "Failed to fetch") })
        );
        errorMsg.appendChild(
          el("div", {
            className: "response-error-hint",
            textContent:
              "This may be caused by CORS restrictions. Ensure the server allows cross-origin requests.",
          })
        );
        responseArea.appendChild(errorMsg);
      }
    }
  }

  // ---------------------------------------------------------------------------
  // JSON syntax highlighting (simple, no deps)
  // ---------------------------------------------------------------------------

  function syntaxHighlightJSON(jsonStr) {
    var fragment = document.createDocumentFragment();

    // Use a regex-based approach for simple highlighting
    var highlighted = jsonStr.replace(
      /("(?:\\.|[^"\\])*")\s*:/g,
      '<span class="json-key">$1</span>:'
    );
    highlighted = highlighted.replace(
      /:\s*("(?:\\.|[^"\\])*")/g,
      ': <span class="json-string">$1</span>'
    );
    highlighted = highlighted.replace(
      /:\s*(\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
      ': <span class="json-number">$1</span>'
    );
    highlighted = highlighted.replace(
      /:\s*(true|false)/g,
      ': <span class="json-boolean">$1</span>'
    );
    highlighted = highlighted.replace(
      /:\s*(null)/g,
      ': <span class="json-null">$1</span>'
    );

    var span = document.createElement("span");
    span.innerHTML = highlighted;
    fragment.appendChild(span);

    return fragment;
  }

  // ---------------------------------------------------------------------------
  // curl generation
  // ---------------------------------------------------------------------------

  function generateCurl(rpc, body, headers, baseURL) {
    var url = baseURL + rpc.connectPath;
    var method = rpc.httpMethod;

    if (method === "GET") {
      var encoded = encodeURIComponent(JSON.stringify(body));
      var getUrl = url + "?encoding=json&message=" + encoded;
      var parts = ["curl"];
      parts.push('"' + getUrl + '"');

      var allHeaders = Object.assign({ "Connect-Protocol-Version": "1" }, headers);
      Object.keys(allHeaders).forEach(function (key) {
        parts.push("-H '" + key + ": " + allHeaders[key] + "'");
      });

      return parts.join(" \\\n  ");
    }

    // POST
    var parts = ["curl -X POST"];
    parts.push("-H 'Content-Type: application/json'");
    parts.push("-H 'Connect-Protocol-Version: 1'");

    Object.keys(headers).forEach(function (key) {
      parts.push("-H '" + key + ": " + headers[key] + "'");
    });

    var jsonBody = JSON.stringify(body);
    parts.push("-d '" + jsonBody + "'");
    parts.push('"' + url + '"');

    return parts.join(" \\\n  ");
  }

  function handleCopyCurl(rpc, panel, btn) {
    var formContainer = panel.querySelector(".form-container");
    var body = {};
    if (formContainer && rpc.request && rpc.request.resolved) {
      body = collectFormData(formContainer, rpc.request.resolved.fields);
    }

    var headers = collectHeaders(panel);
    var baseURL = getBaseURL();
    var curl = generateCurl(rpc, body, headers, baseURL);

    if (navigator.clipboard) {
      navigator.clipboard.writeText(curl).then(function () {
        var originalText = btn.textContent;
        btn.textContent = "Copied!";
        setTimeout(function () {
          btn.textContent = originalText;
        }, 1500);
      });
    } else {
      // Fallback: show in a textarea for manual copy
      var responseArea = panel.querySelector(".response-area");
      responseArea.innerHTML = "";
      var pre = el("pre", { className: "curl-output", textContent: curl });
      responseArea.appendChild(pre);
    }
  }

  // ---------------------------------------------------------------------------
  // Serve mode: SSE hot reload
  // ---------------------------------------------------------------------------

  function connectSSE() {
    var es = new EventSource("/events");
    es.onmessage = function(e) {
      try {
        var data = JSON.parse(e.data);
        if (data.type === "schema-updated") {
          reloadSchema();
        }
      } catch (err) {
        // ignore parse errors
      }
    };
    es.onerror = function() {
      setTimeout(function() {
        es.close();
        connectSSE();
      }, 3000);
    };
  }

  function reloadSchema() {
    fetch("/schema.json")
      .then(function(resp) { return resp.json(); })
      .then(function(schema) {
        window.__CONNECTVIEW_SCHEMA__ = schema;
        var sidebar = document.getElementById("sidebar");
        var mainContent = document.getElementById("main-content");
        if (sidebar) sidebar.innerHTML = "";
        if (mainContent) mainContent.innerHTML = "";
        renderSidebar(schema, sidebar);
        renderMain(schema, mainContent);
        setupFilter(schema, sidebar);
      })
      .catch(function(err) {
        console.error("Failed to reload schema:", err);
      });
  }

  // ---------------------------------------------------------------------------
  // Initialization
  // ---------------------------------------------------------------------------

  function init() {
    var schema = window.__CONNECTVIEW_SCHEMA__;
    if (!schema) {
      console.error("connectview: schema not found at window.__CONNECTVIEW_SCHEMA__");
      return;
    }

    var sidebar = document.getElementById("sidebar");
    var mainContent = document.getElementById("main-content");

    if (!sidebar || !mainContent) {
      console.error("connectview: required DOM elements not found");
      return;
    }

    renderSidebar(schema, sidebar);
    renderMain(schema, mainContent);
    setupFilter(schema, sidebar);

    if (window.__CONNECTVIEW_SERVE_MODE__) {
      connectSSE();
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
