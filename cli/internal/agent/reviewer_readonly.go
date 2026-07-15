package agent

// PermissionsReadOnlyReviewer is the opaque permissions constraint DDx sends
// for reviewer work. Fizeau owns harness selection and decides how the selected
// harness enforces this request; DDx must not maintain a harness-capability
// registry or reject a concrete route locally.
const PermissionsReadOnlyReviewer = "readonly"
