# LLM Draft Plan

## Parsing

- [ ] Parse markdown into node stream
  @id fixture.llm.parse_nodes
  @horizon now
  @accept text:node stream includes headings and checklist items
  @touches internal/compile/parser.go

- [ ] Attach metadata robustly
  @id fixture.llm.attach_metadata
  @horizon next
  @deps fixture.llm.parse_nodes
  @accept cmd:go test ./... -run TestMetadataAttachmentRules
  @touches internal/compile/attach.go

## Diagnostics

- [ ] Emit stable diagnostics
  @id fixture.llm.diag_codes
  @horizon later
  @deps fixture.llm.attach_metadata
  @touches internal/diag/codes.go
