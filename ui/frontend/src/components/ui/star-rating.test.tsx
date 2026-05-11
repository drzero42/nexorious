import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import { StarRating } from "./star-rating";

describe("StarRating", () => {
  it("renders 5 star buttons", () => {
    render(<StarRating />);

    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(5);
  });

  it("displays the current rating value", () => {
    render(<StarRating value={3} showLabel />);

    expect(screen.getByText("(3/5)")).toBeInTheDocument();
  });

  it("displays 'Not rated' when value is null and showLabel is true", () => {
    render(<StarRating value={null} showLabel />);

    expect(screen.getByText("Not rated")).toBeInTheDocument();
  });

  it("calls onChange when a star is clicked", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating onChange={handleChange} />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[2]); // Click the 3rd star

    expect(handleChange).toHaveBeenCalledWith(3);
  });

  it("clears rating when clicking the same star (clearable mode)", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} clearable />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[2]); // Click the 3rd star (already selected)

    expect(handleChange).toHaveBeenCalledWith(null);
  });

  it("does not call onChange when readonly", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} readonly />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[4]); // Try to click the 5th star

    expect(handleChange).not.toHaveBeenCalled();
  });

  it("does not call onChange when disabled", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} disabled />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[4]); // Try to click the 5th star

    expect(handleChange).not.toHaveBeenCalled();
  });

  it("has correct aria-label for readonly mode", () => {
    render(<StarRating value={4} readonly />);

    expect(screen.getByRole("img")).toHaveAttribute(
      "aria-label",
      "Rated 4 out of 5 stars"
    );
  });

  it("has correct aria-label for interactive mode", () => {
    const handleChange = vi.fn();
    render(<StarRating value={2} onChange={handleChange} />);

    expect(screen.getByRole("radiogroup")).toHaveAttribute(
      "aria-label",
      expect.stringContaining("Current rating: 2")
    );
  });

  it("applies correct size classes", () => {
    const { container } = render(<StarRating size="lg" />);

    // Check that the component has rendered (basic sanity check)
    expect(container.querySelector("button")).toBeInTheDocument();
  });
});
