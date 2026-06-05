import type { ReactNode } from 'react';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';

interface CodeHelpAccordionProps {
  /** Stable AccordionItem value, unique within the card. */
  value: string;
  trigger: string;
  children: ReactNode;
}

/** Collapsible "How do I get …?" help section with the shared muted-box layout. */
export function CodeHelpAccordion({ value, trigger, children }: CodeHelpAccordionProps) {
  return (
    <Accordion type="single" collapsible className="w-full">
      <AccordionItem value={value} className="border-none">
        <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
          {trigger}
        </AccordionTrigger>
        <AccordionContent className="text-sm text-muted-foreground">
          <div className="space-y-2 rounded-lg bg-muted/50 p-3">{children}</div>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}
