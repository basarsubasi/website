import type { JSONContent } from "@tiptap/core";
import { postParagraphs } from "./post-layout";

const prefix = "blog-content-v1:";
export const imageWidth = (value: unknown): number => [25, 50, 75, 100].includes(Number(value)) ? Number(value) : 100;
export const imageDimension = (value: unknown): number | null => Number.isSafeInteger(Number(value)) && Number(value) > 0 ? Number(value) : null;
export function imageURL(value: unknown): string {
  if (typeof value !== "string") return "";
  try { const url = new URL(value); return ["https:", "http:"].includes(url.protocol) ? url.href : ""; } catch { return ""; }
}
export function paragraphsToDocument(body: string): JSONContent[] {
  const paragraphs = postParagraphs(body);
  return (paragraphs.length ? paragraphs : [""]).map(paragraph => ({
    type: "paragraph",
    content: paragraph.split("\n").flatMap((line, index): JSONContent[] => [
      ...(index ? [{ type: "hardBreak" }] : []), ...(line ? [{ type: "text", text: line }] : []),
    ]),
  }));
}
export function readPostBody(body: string): JSONContent[] {
  if (body.startsWith(prefix)) {
    try {
      const nodes = JSON.parse(body.slice(prefix.length));
      if (Array.isArray(nodes)) return nodes.map(node => ({
        type: node.type === "blogImage" ? "blogImage" : "paragraph",
        ...(node.type === "blogImage" ? { attrs: { src: imageURL(node.attrs?.src), width: imageWidth(node.attrs?.width), intrinsicWidth: imageDimension(node.attrs?.intrinsicWidth), intrinsicHeight: imageDimension(node.attrs?.intrinsicHeight) } } : {}),
        content: Array.isArray(node.content) ? node.content.flatMap((child: JSONContent) =>
          child.type === "text" && typeof child.text === "string" ? [{ type: "text", text: child.text }] : child.type === "hardBreak" ? [{ type: "hardBreak" }] : []) : [],
      }));
    } catch { /* Old plain text remains readable. */ }
  }
  return paragraphsToDocument(body);
}
export const writePostBody = (nodes: JSONContent[]) => nodes.some(node => node.type === "blogImage")
  ? prefix + JSON.stringify(nodes)
  : nodes.map(blockText).join("\n\n");
export const blockText = (node: JSONContent) => (node.content ?? []).map(child => child.type === "hardBreak" ? "\n" : child.text ?? "").join("");
