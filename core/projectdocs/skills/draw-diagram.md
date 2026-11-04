---
name: draw-diagram
description: Use when an enowX Project Docs document needs a real diagram, such as an ERD, sequence, state, C4, data flow or deployment diagram. Draws it on a canvas from the schema and code, and links it to the document.
---

# Draw a diagram from what is real

A diagram shows what the schema and code say, not what you remember or expect.

## 1. Read the source first

- ERD: the migrations or schema files.
- Sequence, data flow: the handlers along the path, call by call.
- State: the code that changes the status field.
- C4, deployment: the services, the compose or infrastructure files, the imports between parts.

    "$ENOWX_BIN" rag search "where is the orders schema defined"

Note the files you read. For something not built yet, draw from the Technical Spec and say so in the diagram's title. What you can't confirm gets {{Unverified}} in its label.

## 2. Read how a canvas draws

    "$ENOWX_BIN" canvas guide

Read its part on wireframes, diagrams and boards: the sheet comes first and everything goes on it, the grid of 8, at most 7 to 9 boxes per frame, bigger diagrams split into frames.

## 3. Draw it

    "$ENOWX_BIN" canvas new "ERD: Shop" --kind diagram --project .

It prints the canvas's ID. Then the sheet, the title, the boxes, the arrows, one call each:

    "$ENOWX_BIN" canvas shape ID --type rect --id sheet --x 0 --y 0 --w 1200 --h 760 --fill "#ffffff" --stroke "#e2e8f0" --radius 16
    "$ENOWX_BIN" canvas shape ID --type rect --id orders --x 80 --y 120 --w 240 --h 120 --text "orders: id, user_id, total" --fill "#dbeafe" --radius 10
    "$ENOWX_BIN" canvas shape ID --type rect --id users --x 480 --y 120 --w 240 --h 120 --text "users: id, email" --fill "#dbeafe" --radius 10
    "$ENOWX_BIN" canvas shape ID --type arrow --from orders --to users --text "user_id"

Every table, column and call in it is one you read in step 1. Then look at it, and fix what is hard to read:

    "$ENOWX_BIN" canvas screenshot ID /tmp/diagram.png

## 4. Link it to its document

    "$ENOWX_BIN" docs diagram "ERD" --canvas ID --from "db/migrations 001 to 014"

This puts the canvas in the note as an embed, with where it was drawn from and the date checked. The embed in the note is how the owner finds it, so report the document it is in.

## 5. Keep it in step

When the schema or the code it shows changes, in the same turn as that commit: read the source again, change the shapes (canvas shape ID --id orders ...), and run docs diagram again with the new source, so its date says when it was last checked. A diagram you could not bring into step gets {{Unverified}} in the document, with what changed.
