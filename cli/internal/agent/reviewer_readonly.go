package agent

// PermissionsReadOnlyReviewer is the permissions constraint DDx sends for
// reviewer work. Fizeau owns harness selection and decides how the selected
// harness enforces this request; DDx must not maintain a harness-capability
// registry or reject a concrete route locally. The value must stay within
// Fizeau's documented permission vocabulary — subset of {"safe", "supervised",
// "unrestricted"} per fizeau service.go SupportedPermissions — because a value
// outside it matches no routing candidate and ResolveRoute rejects every
// route (ddx-822fb475: "readonly" silently killed all pre-land reviews).
const PermissionsReadOnlyReviewer = "safe"
