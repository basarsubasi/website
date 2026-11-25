import type { JSONContent } from "@tiptap/core";
import { postParagraphs } from "./post-layout";

export type MediaNodeType = "blogImage" | "blogVideo";
export const isMediaNode = (name: unknown): name is MediaNodeType => name === "blogImage" || name === "blogVideo";
const prefix = "blog-content-v1:";
export const mediaWidth = (value: unknown): number => [25, 50, 75, 100].includes(Number(value)) ? Number(value) : 100;
export const mediaDimension = (value: unknown): number | null => Number.isSafeInteger(Number(value)) && Number(value) > 0 ? Number(value) : null;
export function mediaURL(value: unknown): string {
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
function inlineContent(child: JSONContent): JSONContent[] {
  if (child.type === "text" && typeof child.text === "string") return [{ type: "text", text: child.text }];
  if (child.type === "hardBreak") return [{ type: "hardBreak" }];
  return [];
}
export function readPostBody(body: string): JSONContent[] {
  if (body.startsWith(prefix)) {
    try {
      const nodes = JSON.parse(body.slice(prefix.length));
      if (Array.isArray(nodes)) return nodes.map(node => ({
        type: isMediaNode(node.type) ? node.type : "paragraph",
        ...(isMediaNode(node.type) ? { attrs: { src: mediaURL(node.attrs?.src), width: mediaWidth(node.attrs?.width), intrinsicWidth: mediaDimension(node.attrs?.intrinsicWidth), intrinsicHeight: mediaDimension(node.attrs?.intrinsicHeight) } } : {}),
        content: Array.isArray(node.content) ? node.content.flatMap(inlineContent) : [],
      }));
    } catch { /* Old plain text remains readable. */ }
  }
  return paragraphsToDocument(body);
}
export const writePostBody = (nodes: JSONContent[]) => nodes.some(node => isMediaNode(node.type))
  ? prefix + JSON.stringify(nodes)
  : nodes.map(blockText).join("\n\n");
export const blockText = (node: JSONContent) => (node.content ?? []).map(child => child.type === "hardBreak" ? "\n" : child.text ?? "").join("");
