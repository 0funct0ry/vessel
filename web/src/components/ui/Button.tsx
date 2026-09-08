import { forwardRef, type ButtonHTMLAttributes } from "react";

type Variant = "default" | "primary" | "danger";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
}

const VARIANT_CLASSES: Record<Variant, string> = {
  default: "border border-line bg-panel text-text hover:bg-paper",
  primary: "border border-hull bg-hull text-white hover:bg-steel",
  danger: "border border-line bg-panel text-fail hover:bg-paper",
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "default", className = "", ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      className={`rounded px-[11px] py-[5px] text-[13px] font-medium disabled:opacity-45 disabled:cursor-not-allowed ${VARIANT_CLASSES[variant]} ${className}`}
      {...props}
    />
  );
});
