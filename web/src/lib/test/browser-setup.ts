// Browser-project setup. Loads the real app stylesheet so component
// renders have faithful Tailwind layout — tiles have real size, morphed pill
// modes have real visibility — which the visibility/hover/focus assertions rely
// on. Without this, empty utility-styled elements collapse to zero-size and
// visibility checks become meaningless.
import '../../app.css';
